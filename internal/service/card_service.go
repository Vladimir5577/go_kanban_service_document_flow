package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type CardServiceInterface interface {
	CreateCard(ctx context.Context, req dto.CreateCardRequest) (*dto.CardResponse, error)
	DuplicateCard(ctx context.Context, id int64, columnID int64) (*dto.CardResponse, error)
	GetCard(ctx context.Context, id int64) (*model.Card, error)
	GetCardDetail(ctx context.Context, id int64) (*dto.CardResponse, error)
	GetCardStandalone(ctx context.Context, id int64) (*dto.CardStandaloneResponse, error)
	ListTasks(ctx context.Context, f repository.TaskListParams) (*dto.TaskListResponse, error)
	ListTaskCollaborants(ctx context.Context, f repository.TaskCollaborantsParams) (*dto.TaskCollaborantsResponse, error)
	UpdateCard(ctx context.Context, id int64, req dto.UpdateCardRequest) (*dto.CardUpdateResponse, error)
	DeleteCard(ctx context.Context, id int64) error
	UpdateAssignees(ctx context.Context, id int64, userIDs []int64) ([]*dto.CardAssigneeResponse, error)
	MoveCard(ctx context.Context, id int64, columnID *int64, position float64) (*model.CardMove, error)
	ArchiveCard(ctx context.Context, id int64) (bool, error)
	CompleteCard(ctx context.Context, id int64) (*dto.CardCompletionResponse, error)
}

type CardService struct {
	repo              repository.CardRepositoryInterface
	permSvc           *PermissionService
	minioSvc          MinioServiceInterface
	commentRepo       repository.CommentRepositoryInterface
	commentReadRepo   repository.CommentReadRepositoryInterface
	attachmentRepo    repository.AttachmentRepositoryInterface
	labelRepo         repository.LabelRepositoryInterface
	userRepo          repository.UserRepositoryInterface
	columnRepo        repository.ColumnRepositoryInterface
	boardRepo         repository.BoardRepositoryInterface
	projectMemberRepo repository.ProjectMemberRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
	cfg               *config.Config
	History           HistoryLogger
}

const maxActiveCardsPerBoard = 300
const maxChildrenPerCard = 100

func historyOwnerCardID(c *model.Card) int64 {
	if c != nil && c.ParentID != nil {
		return *c.ParentID
	}
	if c == nil {
		return 0
	}
	return c.ID
}

func errIfChildCard(c *model.Card) error {
	if c != nil && c.ParentID != nil {
		return apperr.New(apperr.CodeValidation, "operation not allowed on child card")
	}
	return nil
}

func NewCardService(
	repo repository.CardRepositoryInterface,
	permSvc *PermissionService,
	minioSvc MinioServiceInterface,
	commentRepo repository.CommentRepositoryInterface,
	commentReadRepo repository.CommentReadRepositoryInterface,
	attachmentRepo repository.AttachmentRepositoryInterface,
	labelRepo repository.LabelRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
	boardRepo repository.BoardRepositoryInterface,
	projectMemberRepo repository.ProjectMemberRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
	notificationSvc *KanbanNotificationService,
	cfg *config.Config,
) *CardService {
	return &CardService{
		repo:              repo,
		permSvc:           permSvc,
		minioSvc:          minioSvc,
		commentRepo:       commentRepo,
		commentReadRepo:   commentReadRepo,
		attachmentRepo:    attachmentRepo,
		labelRepo:         labelRepo,
		userRepo:          userRepo,
		columnRepo:        columnRepo,
		boardRepo:         boardRepo,
		projectMemberRepo: projectMemberRepo,
		realtimePublisher: realtimePublisher,
		notificationSvc:   notificationSvc,
		cfg:               cfg,
	}
}

func (s *CardService) CreateCard(ctx context.Context, req dto.CreateCardRequest) (*dto.CardResponse, error) {
	if (req.ColumnID == nil) == (req.ParentID == nil) {
		return nil, apperr.New(apperr.CodeValidation, "exactly one of column_id or parent_id is required")
	}
	if req.ParentID != nil {
		return s.createChildCard(ctx, req)
	}
	return s.createRootCard(ctx, req)
}

// createdCardResponse — ответ на создание и копию. Метки, исполнители и
// комментарии у новой карточки пусты по-настоящему, поэтому пустые списки
// MapCardResponse здесь правдивы, и перечитывать карточку незачем.
func createdCardResponse(c *model.Card, boardID int64, columnTitle string) *dto.CardResponse {
	resp := dto.MapCardResponse(c)
	resp.BoardID = boardID
	resp.ColumnTitle = columnTitle
	return resp
}

// createChildCard создаёт пустую подзадачу: только заголовок и позиция,
// исполнитель назначается потом. Запросы: контекст родителя, счётчик
// подзадач, INSERT, история; в фоне — счётчик у родителя.
func (s *CardService) createChildCard(ctx context.Context, req dto.CreateCardRequest) (*dto.CardResponse, error) {
	if req.Description != nil || req.DueDate != nil || req.Priority != nil || req.BorderColor != nil {
		return nil, apperr.New(apperr.CodeValidation, "child card accepts only title and position")
	}

	// Один запрос вместо GetCard + GetProjectIDByCard + RequireRole + GetColumn:
	// отдаёт проект, доску и родительство разом.
	parentID := *req.ParentID
	acc, err := s.permSvc.RequireCardRole(ctx, parentID, RoleEditor)
	if err != nil {
		return nil, err
	}
	if acc.ParentID != nil {
		return nil, apperr.New(apperr.CodeValidation, "parent must be a root card")
	}

	childCount, lastPosition, err := s.repo.GetChildStats(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if childCount >= maxChildrenPerCard {
		return nil, apperr.New(apperr.CodeValidation, "maximum number of subtasks (100) per card reached")
	}

	c := &model.Card{
		Title:       req.Title,
		ParentID:    req.ParentID,
		CreatedByID: currentUserID(ctx),
	}
	if req.Position != nil {
		c.Position = *req.Position
	} else {
		c.Position = lastPosition + 65536
	}

	created, err := s.repo.CreateCard(ctx, 0, c)
	if err != nil {
		return nil, err
	}

	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "card.created",
		EntityType:  "card",
		EntityID:    created.ID,
		CardID:      parentID,
		EntityTitle: created.Title,
		EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, parentID),
	})
	s.publishParentChecklist(ctx, acc.BoardID, parentID)
	return createdCardResponse(created, acc.BoardID, acc.ColumnTitle), nil
}

// createRootCard создаёт пустую карточку: заголовок, описание, срок, приоритет
// и цвет; исполнитель и метки добавляются потом. Запросы: контекст колонки с
// правами и позицией, лимит доски, INSERT, история; в фоне — админы для
// уведомления. Ответ и realtime собираются из того, что вернул INSERT.
func (s *CardService) createRootCard(ctx context.Context, req dto.CreateCardRequest) (*dto.CardResponse, error) {
	columnID := *req.ColumnID
	acc, err := s.permSvc.RequireColumnRole(ctx, columnID, RoleEditor)
	if err != nil {
		return nil, err
	}

	activeCardsCount, err := s.repo.CountActiveCardsByBoard(ctx, acc.BoardID)
	if err != nil {
		return nil, err
	}
	if activeCardsCount >= maxActiveCardsPerBoard {
		return nil, apperr.New(apperr.CodeBoardCardLimitReached, "maximum number of cards (300) on board reached")
	}

	c := &model.Card{
		Title:       req.Title,
		ColumnID:    columnID,
		Description: req.Description,
		DueDate:     normalizeTimePtr(req.DueDate),
		Priority:    req.Priority,
		BorderColor: req.BorderColor,
		CreatedByID: currentUserID(ctx),
	}
	if req.Position != nil {
		c.Position = *req.Position
	} else {
		// Наверх колонки: FIRST/2, как computePosition(undefined, next) = next/2
		// на фронте. Вычитание фиксированного шага уходило в 0 и минус на первом
		// же добавлении и ломало расчёт перемещений. Позицию посчитал контекст.
		c.Position = acc.PrependPosition
	}
	created, err := s.repo.CreateCard(ctx, columnID, c)
	if err != nil {
		return nil, err
	}

	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "card.created",
		EntityType:  "card",
		EntityID:    created.ID,
		CardID:      created.ID,
		EntityTitle: created.Title,
		EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, created.ID),
	})
	if s.realtimePublisher != nil {
		event := s.realtimePublisher.BuildCreatedCard(created, &model.Column{ID: columnID, BoardID: acc.BoardID})
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			return s.realtimePublisher.PublishCardCreated(ctx, acc.BoardID, event, realtimeSenderID(ctx))
		})
	}
	if s.notificationSvc != nil {
		actorID := derefInt64(currentUserID(ctx))
		runDetached(ctx, notifyTimeout, "failed to notify kanban card created", func(ctx context.Context) error {
			s.notificationSvc.NotifyCardCreated(ctx, acc.ProjectID, acc.BoardID, acc.BoardTitle, created.ID, actorID, created.Title)
			return nil
		})
	}
	return createdCardResponse(created, acc.BoardID, acc.ColumnTitle), nil
}

func (s *CardService) GetCardDetail(ctx context.Context, id int64) (*dto.CardResponse, error) {
	acc, err := s.permSvc.RequireCardRole(ctx, id, RoleViewer)
	if err != nil {
		return nil, err
	}
	resp, _, err := s.cardDetail(ctx, id, acc)
	return resp, err
}

// cardDetail собирает карточку, когда доступ уже проверен и контекст известен.
// Вторым значением отдаёт метки доски: GetCardStandalone нужен их полный
// список, и читать его второй раз за тот же запрос незачем.
func (s *CardService) cardDetail(ctx context.Context, id int64, acc CardAccess) (*dto.CardResponse, []model.Label, error) {
	card, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	resp := dto.MapCardResponse(card)

	if card.ParentID == nil {
		children, err := s.repo.GetChildCards(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		resp.Children = make([]*dto.CardChildResponse, 0, len(children))
		for i := range children {
			child := children[i]
			resp.Children = append(resp.Children, &dto.CardChildResponse{
				ID:          child.ID,
				Title:       child.Title,
				Position:    child.Position,
				CompletedAt: child.CompletedAt,
				AssigneeIDs: child.AssigneeIDs,
			})
			resp.ChecklistTotal++
			if child.CompletedAt != nil {
				resp.ChecklistDone++
			}
		}
	}

	// Fetch Comments
	comments, err := s.commentRepo.GetComments(ctx, id)
	if err != nil {
		return nil, nil, err
	}

	var userIDs []int64
	for i := range comments {
		userIDs = append(userIDs, comments[i].AuthorID)
	}
	for _, aid := range card.AssigneeIDs {
		userIDs = append(userIDs, aid)
	}
	for _, child := range resp.Children {
		userIDs = append(userIDs, child.AssigneeIDs...)
	}
	// Include creators for createdBy / completedBy enrichment
	if card.CreatedByID != nil {
		userIDs = append(userIDs, *card.CreatedByID)
	}
	if card.CompletedByID != nil {
		userIDs = append(userIDs, *card.CompletedByID)
	}

	var allAttachments []model.Attachment
	attsCard, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "card")
	if err != nil {
		return nil, nil, err
	}
	allAttachments = append(allAttachments, attsCard...)

	attsDesc, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "description")
	if err != nil {
		return nil, nil, err
	}
	allAttachments = append(allAttachments, attsDesc...)

	chatAttachments, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "chat")
	if err != nil {
		return nil, nil, err
	}
	allAttachments = append(allAttachments, chatAttachments...)

	resp.CommentsCount = len(comments) + len(chatAttachments)
	for i := range allAttachments {
		if allAttachments[i].AuthorID != nil {
			userIDs = append(userIDs, *allAttachments[i].AuthorID)
		}
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	userMap := make(map[int64]*model.User)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	for i := range comments {
		if u, ok := userMap[comments[i].AuthorID]; ok {
			comments[i].AuthorName = dto.UserDisplayName(*u)
		}
	}
	resp.Comments = dto.MapCommentsResponse(comments)

	// Точка старта ленты: фронт прокручивает к первому непрочитанному по этому
	// числу, и только потом шлёт новую отметку. Ноль — карточку ещё не открывали.
	if uid := currentUserID(ctx); uid != nil {
		mark, err := s.commentReadRepo.GetMark(ctx, id, *uid)
		if err != nil {
			return nil, nil, err
		}
		resp.LastReadCommentID = mark
	}

	resp.Attachments = dto.MapAttachmentsResponse(s.cfg, allAttachments)
	for i, att := range resp.Attachments {
		if att.AuthorID != nil {
			if u, ok := userMap[*att.AuthorID]; ok {
				name := dto.UserDisplayName(*u)
				resp.Attachments[i].AuthorName = &name
			}
		}
	}

	// Доска и заголовок колонки пришли вместе с проверкой прав — отдельный
	// GetColumn для этого больше не нужен.
	resp.BoardID = acc.BoardID
	resp.ColumnTitle = acc.ColumnTitle

	labels, err := s.labelRepo.GetLabels(ctx, acc.BoardID)
	if err != nil {
		return nil, nil, err
	}
	labelMap := make(map[int64]*dto.LabelResponse, len(labels))
	for i := range labels {
		label := dto.MapLabelResponse(&labels[i])
		labelMap[label.ID] = label
	}
	for _, labelID := range card.LabelIDs {
		if label, ok := labelMap[labelID]; ok {
			resp.Labels = append(resp.Labels, label)
		}
	}

	for _, uid := range card.AssigneeIDs {
		if u, ok := userMap[uid]; ok {
			name := dto.UserDisplayName(*u)
			resp.Assignees = append(resp.Assignees, &dto.CardAssigneeResponse{
				ID:        u.ID,
				Name:      name,
				AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
			})
		}
	}

	// Populate rich creator objects (createdBy / completedBy)
	if card.CreatedByID != nil {
		if u, ok := userMap[*card.CreatedByID]; ok {
			resp.CreatedBy = &dto.CardUserResponse{
				ID:        u.ID,
				Firstname: u.Firstname,
				Lastname:  u.Lastname,
				AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
			}
		}
	}
	if card.CompletedByID != nil {
		if u, ok := userMap[*card.CompletedByID]; ok {
			resp.CompletedBy = &dto.CardUserResponse{
				ID:        u.ID,
				Firstname: u.Firstname,
				Lastname:  u.Lastname,
				AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
			}
		}
	}

	return resp, labels, nil
}

func (s *CardService) GetCardStandalone(ctx context.Context, id int64) (*dto.CardStandaloneResponse, error) {
	acc, err := s.permSvc.RequireCardRole(ctx, id, RoleViewer)
	if err != nil {
		return nil, err
	}

	card, labels, err := s.cardDetail(ctx, id, acc)
	if err != nil {
		return nil, err
	}

	projectID := acc.ProjectID
	role := acc.Role

	columns, err := s.columnRepo.GetColumnsByBoard(ctx, card.BoardID)
	if err != nil {
		return nil, err
	}
	columnResp := make([]*dto.CardStandaloneColumnResponse, 0, len(columns))
	for i := range columns {
		columnResp = append(columnResp, &dto.CardStandaloneColumnResponse{
			ID:          columns[i].ID,
			Title:       columns[i].Title,
			HeaderColor: columns[i].HeaderColor,
			Position:    columns[i].Position,
		})
	}

	board, err := s.boardRepo.GetBoard(ctx, card.BoardID)
	if err != nil {
		return nil, err
	}

	// Владелец известен из проверки прав — отдельный GetProject не нужен.
	members, err := s.projectMemberRepo.GetMembers(ctx, projectID)
	if err != nil {
		return nil, err
	}
	members = ensureOwnerMember(members, acc.OwnerID, projectID)

	userIDs := make([]int64, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, m.UserID)
	}
	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[int64]*model.User, len(users))
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}

	memberResp := make([]*dto.MemberResponse, 0, len(members))
	for _, m := range members {
		item := &dto.MemberResponse{
			UserID:  m.UserID,
			Role:    m.Role,
			IsOwner: m.UserID == acc.OwnerID,
		}
		if u, ok := userMap[m.UserID]; ok {
			item.Login = u.Login
			item.Lastname = u.Lastname
			item.Firstname = u.Firstname
			item.Patronymic = u.Patronymic
			item.AvatarUrl = dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail)
		}
		memberResp = append(memberResp, item)
	}

	// labels уже прочитаны в cardDetail — это те же метки той же доски.

	currUser, _ := middleware.GetUser(ctx)
	isOwner := acc.IsOwner(currUser.ID)

	return &dto.CardStandaloneResponse{
		Card:           card,
		ProjectID:      projectID,
		MemberRole:     string(role),
		IsOwner:        isOwner,
		IsProjectAdmin: role == RoleAdmin || isOwner,
		DoneColumnID:   board.DoneColumnID,
		Columns:        columnResp,
		Members:        memberResp,
		Labels:         dto.MapLabelsResponse(labels),
	}, nil
}

func (s *CardService) GetCard(ctx context.Context, id int64) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	card, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	return card, nil
}

func (s *CardService) ListTasks(ctx context.Context, f repository.TaskListParams) (*dto.TaskListResponse, error) {
	userID := currentUserID(ctx)
	if userID == nil {
		return nil, apperr.ErrUnauthorized
	}
	f.ViewerID = *userID

	rows, total, err := s.repo.ListTasks(ctx, f)
	if err != nil {
		return nil, err
	}
	items := mapTaskListItems(rows)
	if err := s.attachTaskAssignees(ctx, items); err != nil {
		return nil, err
	}
	return &dto.TaskListResponse{Items: items, Total: total}, nil
}

func (s *CardService) attachTaskAssignees(ctx context.Context, items []*dto.TaskListItem) error {
	if len(items) == 0 {
		return nil
	}
	cardIDs := make([]int64, len(items))
	for i, item := range items {
		cardIDs[i] = item.ID
	}
	byCard, err := s.repo.GetAssigneesByCardIDs(ctx, cardIDs)
	if err != nil {
		return err
	}
	userIDs := make([]int64, 0, len(byCard))
	seen := make(map[int64]struct{}, len(byCard))
	for _, ids := range byCard {
		if len(ids) == 0 {
			continue
		}
		if _, ok := seen[ids[0]]; ok {
			continue
		}
		seen[ids[0]] = struct{}{}
		userIDs = append(userIDs, ids[0])
	}
	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return err
	}
	attachAssigneesToTaskItems(items, byCard, users)
	return nil
}

func (s *CardService) ListTaskCollaborants(ctx context.Context, f repository.TaskCollaborantsParams) (*dto.TaskCollaborantsResponse, error) {
	userID := currentUserID(ctx)
	if userID == nil {
		return nil, apperr.ErrUnauthorized
	}
	f.ViewerID = *userID

	rows, total, err := s.repo.ListTaskCollaborants(ctx, f)
	if err != nil {
		return nil, err
	}
	return &dto.TaskCollaborantsResponse{Items: dto.MapUsersResponse(rows), Total: total}, nil
}

// UpdateCard правит собственные поля карточки. Запросы: контекст с правами,
// строка карточки ради старых значений, узкий UPDATE, история. Доску для
// ссылки истории и realtime приносит контекст, ответ собирается из строки.
func (s *CardService) UpdateCard(ctx context.Context, id int64, req dto.UpdateCardRequest) (*dto.CardUpdateResponse, error) {
	acc, err := s.permSvc.RequireCardRole(ctx, id, RoleEditor)
	if err != nil {
		return nil, err
	}

	c, err := s.repo.GetCardRow(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	if c.ParentID != nil && (req.HasDescription || req.HasDueDate || req.HasPriority || req.HasBorderColor) {
		return nil, apperr.New(apperr.CodeValidation, "child card accepts only title")
	}
	var titleChanged, descChanged, dueChanged, priorityChanged, colorChanged bool
	var oldTitle, newTitle *string
	var oldDescription *string
	var oldDue, newDue *string
	var oldPriorityKey, newPriorityKey string
	var oldColorKey, newColorKey string

	if req.Title != nil {
		trimmedTitle := strings.TrimSpace(*req.Title)
		if trimmedTitle != "" && c.Title != trimmedTitle {
			titleChanged = true
			tOld := c.Title
			tNew := trimmedTitle
			oldTitle, newTitle = &tOld, &tNew
			c.Title = tNew
		}
	}

	if req.HasDescription {
		oldDescription = c.Description
		if !sameOptionalString(oldDescription, req.Description) {
			descChanged = true
			c.Description = req.Description
		}
	}

	if req.HasDueDate {
		if !sameOptionalTime(c.DueDate, req.DueDate) {
			dueChanged = true
			if c.DueDate != nil {
				dStr := c.DueDate.In(helper.MoscowLocation()).Format("02.01.2006 15:04")
				oldDue = &dStr
			}
			if req.DueDate != nil {
				dStr := req.DueDate.In(helper.MoscowLocation()).Format("02.01.2006 15:04")
				newDue = &dStr
			}
			c.DueDate = normalizeTimePtr(req.DueDate)
		}
	}

	if req.HasPriority {
		normalizedPriority := normalizeCardPriority(req.Priority)
		if !sameOptionalString(c.Priority, normalizedPriority) {
			priorityChanged = true
			oldPriorityKey = historyText(c.Priority)
			newPriorityKey = historyText(normalizedPriority)
			c.Priority = normalizedPriority
		}
	}

	if req.HasBorderColor {
		normalizedColor := normalizeCardBorderColor(req.BorderColor)
		if !sameOptionalString(c.BorderColor, normalizedColor) {
			colorChanged = true
			oldColorKey = historyText(c.BorderColor)
			newColorKey = historyText(normalizedColor)
			c.BorderColor = normalizedColor
		}
	}

	if !titleChanged && !descChanged && !dueChanged && !priorityChanged && !colorChanged {
		return s.cardUpdateResponse(ctx, c, acc.BoardID)
	}

	updatedCard, err := s.repo.UpdateCardFields(ctx, c)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	action := cardFieldAction(titleChanged, descChanged, dueChanged, priorityChanged, colorChanged)
	before, after := "", ""
	switch action {
	case "card.updated.renamed":
		before, after = historyText(oldTitle), historyText(newTitle)
	case "card.updated.description":
		before, after = historyText(oldDescription), historyText(req.Description)
	case "card.updated.due_date":
		before, after = historyText(oldDue), historyText(newDue)
	case "card.updated.priority":
		before, after = oldPriorityKey, newPriorityKey
	case "card.updated.color":
		before, after = oldColorKey, newColorKey
	}
	ownerCardID := historyOwnerCardID(updatedCard)
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      action,
		EntityType:  "card",
		EntityID:    id,
		CardID:      ownerCardID,
		EntityTitle: updatedCard.Title,
		EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, ownerCardID),
		Before:      before,
		After:       after,
	})
	if s.realtimePublisher != nil && updatedCard.ParentID == nil {
		patch := map[string]any{}
		if titleChanged {
			patch["title"] = updatedCard.Title
		}
		if priorityChanged {
			patch["priority"] = updatedCard.Priority
		}
		if dueChanged {
			patch["dueDate"] = formatRealtimeTime(updatedCard.DueDate)
		}
		if colorChanged {
			patch["borderColor"] = updatedCard.BorderColor
		}
		if len(patch) > 0 {
			patch["id"] = updatedCard.ID
			patch["updatedAt"] = formatRealtimeTimeValue(updatedCard.UpdatedAt)
			s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
				return s.realtimePublisher.PublishCardUpdated(ctx, acc.BoardID, patch, realtimeSenderID(ctx))
			})
		}
	}
	return s.cardUpdateResponse(ctx, updatedCard, acc.BoardID)
}

// cardUpdateResponse собирает ответ правки из строки карточки. Подзадаче
// дописывает исполнителя: фронт показывает его в списке подзадач.
func (s *CardService) cardUpdateResponse(ctx context.Context, c *model.Card, boardID int64) (*dto.CardUpdateResponse, error) {
	resp := &dto.CardUpdateResponse{
		ID:          c.ID,
		Title:       c.Title,
		Description: c.Description,
		Position:    c.Position,
		DueDate:     c.DueDate,
		Priority:    c.Priority,
		BorderColor: c.BorderColor,
		ParentID:    c.ParentID,
		BoardID:     boardID,
		CompletedAt: c.CompletedAt,
		UpdatedAt:   c.UpdatedAt,
	}
	if c.ParentID == nil {
		columnID := c.ColumnID
		resp.ColumnID = &columnID
		return resp, nil
	}

	byCard, err := s.repo.GetAssigneesByCardIDs(ctx, []int64{c.ID})
	if err != nil {
		return nil, err
	}
	resp.AssigneeIDs = byCard[c.ID]
	if len(resp.AssigneeIDs) == 0 {
		return resp, nil
	}
	users, err := s.userRepo.GetUsersByIDs(ctx, resp.AssigneeIDs)
	if err != nil {
		return nil, err
	}
	for i := range users {
		resp.Assignees = append(resp.Assignees, &dto.CardAssigneeResponse{
			ID:        users[i].ID,
			Name:      dto.UserDisplayName(users[i]),
			AvatarUrl: dto.UserAvatarURL(s.cfg, users[i].AvatarName, dto.AvatarSizeThumbnail),
		})
	}
	return resp, nil
}

// DeleteCard мягко удаляет карточку вместе с подзадачами или одну подзадачу.
// Карточку удаляет админ, подзадачу — редактор. Запросы: контекст с правами,
// удаление, история; доску для realtime приносит контекст.
func (s *CardService) DeleteCard(ctx context.Context, id int64) error {
	acc, err := s.permSvc.RequireCardRole(ctx, id, RoleEditor)
	if err != nil {
		return err
	}
	if acc.ParentID == nil && !hasRole(acc.Role, RoleAdmin) {
		return accessDenied()
	}

	if err := s.repo.DeleteCard(ctx, id); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperr.New(apperr.CodeValidation, "Нельзя удалить задачу, пока в ней есть прикрепленные данные.")
		}
		return withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	// Удалённую карточку не открыть — ссылка ведёт в проект; подзадача — в родителя.
	ownerID := id
	link := historyProjectPath(acc.ProjectID)
	if acc.ParentID != nil {
		ownerID = *acc.ParentID
		link = historyTaskPath(acc.ProjectID, acc.BoardID, ownerID)
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "card.deleted",
		EntityType:  "card",
		EntityID:    id,
		CardID:      ownerID,
		EntityTitle: acc.CardTitle,
		EntityLink:  link,
	})
	if acc.ParentID != nil {
		s.publishParentChecklist(ctx, acc.BoardID, *acc.ParentID)
		return nil
	}
	if s.realtimePublisher != nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			return s.realtimePublisher.PublishCardDeleted(ctx, acc.BoardID, id, realtimeSenderID(ctx))
		})
	}
	return nil
}

// UpdateAssignees ставит или снимает исполнителя (не больше одного) и отдаёт
// новый состав для ответа. Запросы: контекст с правами, кандидат с членством,
// замена исполнителей, история. Доска, заголовок и владелец приезжают с
// контекстом, исполнитель — с проверкой, поэтому ни ответу, ни realtime, ни
// уведомлению перечитывать карточку не нужно.
func (s *CardService) UpdateAssignees(ctx context.Context, id int64, userIDs []int64) ([]*dto.CardAssigneeResponse, error) {
	if len(userIDs) > 1 {
		return nil, apperr.New(apperr.CodeValidation, "maximum 1 assignee allowed")
	}

	acc, err := s.permSvc.RequireCardRole(ctx, id, RoleEditor)
	if err != nil {
		return nil, err
	}

	var assignee *model.User
	if len(userIDs) == 1 {
		u, isMember, err := s.userRepo.GetProjectAssignee(ctx, acc.ProjectID, userIDs[0])
		if errors.Is(err, apperr.ErrNotFound) {
			return nil, apperr.New(apperr.CodeUserNotFound, "user not found")
		}
		if err != nil {
			return nil, err
		}
		if !isMember && u.ID != acc.OwnerID {
			return nil, apperr.New(apperr.CodeUserNotProjectMember, "user is not project member")
		}
		assignee = u
	}

	oldIDs, err := s.repo.ReplaceCardAssignees(ctx, id, userIDs)
	if err != nil {
		return nil, err
	}

	// Подзадача пишется в историю и уведомляет от имени родителя.
	ownerCardID := id
	if acc.ParentID != nil {
		ownerCardID = *acc.ParentID
	}
	action := "card.assignees"
	if len(oldIDs) > 0 && len(userIDs) == 0 {
		action = "card.assignee_removed"
	} else if len(oldIDs) == 0 && len(userIDs) > 0 {
		action = "card.assignee_added"
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      action,
		EntityType:  "card",
		EntityID:    id,
		CardID:      ownerCardID,
		EntityTitle: acc.CardTitle,
		EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, ownerCardID),
		Before:      historyIDsJSON(oldIDs),
		After:       historyIDsJSON(userIDs),
	})

	var assignees []*dto.CardAssigneeResponse
	realtimeAssignees := []map[string]any{}
	if assignee != nil {
		assignees = []*dto.CardAssigneeResponse{{
			ID:        assignee.ID,
			Name:      dto.UserDisplayName(*assignee),
			AvatarUrl: dto.UserAvatarURL(s.cfg, assignee.AvatarName, dto.AvatarSizeThumbnail),
		}}
		realtimeAssignees = append(realtimeAssignees, formatRealtimeAssignee(s.cfg, *assignee))
	}

	if s.realtimePublisher != nil && acc.ParentID == nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			return s.realtimePublisher.PublishCardUpdated(ctx, acc.BoardID, map[string]any{
				"id":        id,
				"assignees": realtimeAssignees,
			}, realtimeSenderID(ctx))
		})
	}

	if s.notificationSvc != nil && assignee != nil {
		actorID := derefInt64(currentUserID(ctx))
		runDetached(ctx, notifyTimeout, "failed to notify kanban task assigned", func(ctx context.Context) error {
			s.notificationSvc.NotifyTaskAssigned(ctx, acc.ProjectID, acc.BoardID, acc.BoardTitle, ownerCardID, actorID, assignee.ID, acc.CardTitle, acc.ParentID != nil)
			return nil
		})
	}
	return assignees, nil
}

func (s *CardService) MoveCard(ctx context.Context, id int64, columnID *int64, position float64) (*model.CardMove, error) {
	acc, err := s.permSvc.RequireCardRole(ctx, id, RoleEditor)
	if err != nil {
		return nil, err
	}
	// Родительство уже приехало вместе с проверкой прав — читать карточку
	// целиком (а с ней assignees и labels, которые тут не нужны) незачем.
	if acc.ParentID != nil {
		if columnID != nil {
			return nil, apperr.New(apperr.CodeValidation, "child card cannot change column")
		}
		updated, err := s.repo.MoveChildCard(ctx, id, position)
		if err != nil {
			return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
		}
		// У подзадачи нет колонки, поэтому Before/After — только позиция.
		// applyHistoryUndo разбирает такой формат отдельной веткой.
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   acc.ProjectID,
			Action:      "card.moved",
			EntityType:  "card",
			EntityID:    id,
			CardID:      *acc.ParentID,
			EntityTitle: updated.Title,
			EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, *acc.ParentID),
			Before:      historyPos(updated.FromPosition),
			After:       historyPos(updated.Position),
		})
		return updated, nil
	}
	if columnID == nil || *columnID == 0 {
		return nil, apperr.New(apperr.CodeColumnIDAndPositionRequired, "column_id and position required")
	}

	targetColumn, err := s.columnRepo.GetColumn(ctx, *columnID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeColumnNotFound)
	}
	if targetColumn.BoardID != acc.BoardID {
		return nil, apperr.New(apperr.CodeColumnNotFound, "column not found")
	}

	move, err := s.repo.MoveCard(ctx, id, *columnID, position)
	if err != nil {
		return nil, err
	}
	columnChanged := move.FromColumnID != *columnID

	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "card.moved",
		EntityType:  "card",
		EntityID:    id,
		CardID:      id,
		EntityTitle: move.Title,
		// Ссылку собираем сами: без неё HistoryService пойдёт её вычислять и
		// перечитает карточку вместе с исполнителями и метками, а следом колонку —
		// четыре запроса ради строки, все части которой уже лежат здесь.
		EntityLink: historyTaskPath(acc.ProjectID, acc.BoardID, id),
		Before:     historyPlacement(move.FromColumnID, move.FromPosition),
		After:      historyPlacement(*columnID, move.Position),
	})

	if s.realtimePublisher != nil {
		patch := map[string]any{
			"id":        move.ID,
			"position":  move.Position,
			"updatedAt": formatRealtimeTimeValue(move.UpdatedAt),
		}
		if columnChanged {
			patch["columnId"] = targetColumn.ID
			patch["columnTitle"] = targetColumn.Title
			patch["status"] = strconv.FormatInt(targetColumn.ID, 10)
		}
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			senderID := realtimeSenderID(ctx)
			if err := s.realtimePublisher.PublishCardUpdated(ctx, targetColumn.BoardID, patch, senderID); err != nil {
				return err
			}
			return s.publishRebalanced(ctx, targetColumn.BoardID, move.ID, move.Rebalanced, senderID)
		})
	}

	// Notify on column change (moved)
	if columnChanged && s.notificationSvc != nil {
		actorID := derefInt64(currentUserID(ctx))
		runDetached(ctx, notifyTimeout, "failed to notify kanban task moved", func(ctx context.Context) error {
			// source and target are guaranteed to be on the same board
			s.notificationSvc.NotifyTaskMoved(ctx, acc.ProjectID, acc.BoardID, acc.BoardTitle, id, actorID, move.Title, acc.ColumnTitle, targetColumn.Title)
			return nil
		})
	}

	return move, nil
}

// ArchiveCard переключает карточку между архивом и доской и отдаёт новое
// состояние. Три запроса: контекст с правами и текущим флагом, узкий UPDATE,
// запись в историю. Доска приезжает с контекстом, поэтому ни ссылке истории,
// ни событию realtime не нужно перечитывать карточку и колонку.
func (s *CardService) ArchiveCard(ctx context.Context, id int64) (bool, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, id, RoleAdmin)
	if err != nil {
		return false, err
	}

	archived := !acc.IsArchived
	action := "card.restored"
	link := historyTaskPath(acc.ProjectID, acc.BoardID, id)
	var archivedAt *time.Time
	var archivedByID *int64
	if archived {
		action = "card.archived"
		link = historyArchivePath(acc.ProjectID, acc.BoardID)
		now := s.cfg.Clock.Now()
		archivedAt = &now
		archivedByID = currentUserID(ctx)
	}

	title, err := s.repo.SetCardArchived(ctx, id, archived, archivedAt, archivedByID)
	if err != nil {
		return false, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      action,
		EntityType:  "card",
		EntityID:    id,
		CardID:      id,
		EntityTitle: title,
		EntityLink:  link,
	})

	// Архивация убирает карточку с доски, восстановление возвращает её обратно.
	// Без события у чужих вкладок она висит (или не появляется) до перезагрузки.
	if s.realtimePublisher != nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			senderID := realtimeSenderID(ctx)
			if archived {
				return s.realtimePublisher.PublishCardDeleted(ctx, acc.BoardID, id, senderID)
			}
			created, err := s.realtimePublisher.BuildRestoredCard(ctx, id, acc.BoardID)
			if err != nil {
				return err
			}
			return s.realtimePublisher.PublishCardCreated(ctx, acc.BoardID, created, senderID)
		})
	}
	return archived, nil
}

// CompleteCard переключает выполнение и отдаёт то, что поменялось; корневую
// карточку при завершении уносит наверх колонки «Готово». Колонку, позицию,
// флаг выполнения и колонку «Готово» приносит контекст с правами, поэтому
// карточку, колонку и доску отдельно не читаем, а ссылки истории собираем сами.
func (s *CardService) CompleteCard(ctx context.Context, id int64) (*dto.CardCompletionResponse, error) {
	acc, err := s.permSvc.RequireCardRole(ctx, id, RoleEditor)
	if err != nil {
		return nil, err
	}

	completing := acc.CompletedAt == nil
	action := "card.reopened"
	var completedAt *time.Time
	var completedByID *int64
	if completing {
		action = "card.completed"
		now := s.cfg.Clock.Now()
		completedAt = &now
		completedByID = currentUserID(ctx)
	}

	updatedAt, err := s.repo.SetCardCompleted(ctx, id, completedAt, completedByID)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	// Подзадача пишется в историю от имени родителя.
	ownerCardID := id
	if acc.ParentID != nil {
		ownerCardID = *acc.ParentID
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      action,
		EntityType:  "card",
		EntityID:    id,
		CardID:      ownerCardID,
		EntityTitle: acc.CardTitle,
		EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, ownerCardID),
	})

	resp := &dto.CardCompletionResponse{
		ID:            id,
		Title:         acc.CardTitle,
		Position:      acc.Position,
		BoardID:       acc.BoardID,
		ParentID:      acc.ParentID,
		CompletedAt:   completedAt,
		CompletedByID: completedByID,
		UpdatedAt:     updatedAt,
	}
	if acc.ParentID != nil {
		// Подзадаче фронт показывает исполнителя — он нужен в ответе.
		byCard, err := s.repo.GetAssigneesByCardIDs(ctx, []int64{id})
		if err != nil {
			return nil, err
		}
		resp.AssigneeIDs = byCard[id]
	} else {
		columnID := acc.ColumnID
		resp.ColumnID = &columnID
		resp.ColumnTitle = acc.ColumnTitle
	}

	var rebalanced []model.CardPosition
	moved := false
	if completing && acc.ParentID == nil && acc.DoneColumnID != nil && *acc.DoneColumnID != acc.ColumnID {
		// Автоперенос — побочный эффект: его сбой не отменяет завершения.
		if target, err := s.repo.ColumnPrependTarget(ctx, *acc.DoneColumnID); err == nil {
			if move, err := s.repo.MoveCard(ctx, id, *acc.DoneColumnID, target.Position); err == nil && move != nil {
				moved = true
				rebalanced = move.Rebalanced
				resp.ColumnID = &move.ToColumnID
				resp.ColumnTitle = target.Title
				resp.Position = move.Position
				resp.UpdatedAt = move.UpdatedAt
				appendHistory(s.History, ctx, model.HistoryWrite{
					ProjectID:   acc.ProjectID,
					Action:      "card.moved",
					EntityType:  "card",
					EntityID:    id,
					CardID:      id,
					EntityTitle: acc.CardTitle,
					EntityLink:  historyTaskPath(acc.ProjectID, acc.BoardID, id),
					Before:      historyPlacement(acc.ColumnID, acc.Position),
					After:       historyPlacement(move.ToColumnID, move.Position),
				})
			}
		}
	}

	if err := s.fillCompletionUsers(ctx, resp); err != nil {
		return nil, err
	}

	if acc.ParentID != nil {
		s.publishParentChecklist(ctx, acc.BoardID, *acc.ParentID)
		return resp, nil
	}
	if s.realtimePublisher != nil {
		patch := map[string]any{
			"id":            id,
			"completedAt":   formatRealtimeTime(completedAt),
			"completedById": completedByID,
			"columnId":      *resp.ColumnID,
			"position":      resp.Position,
			"updatedAt":     formatRealtimeTimeValue(resp.UpdatedAt),
		}
		if moved {
			// Как у обычного перемещения: доска раскладывает карточки по статусу.
			patch["columnTitle"] = resp.ColumnTitle
			patch["status"] = strconv.FormatInt(*resp.ColumnID, 10)
		}
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			senderID := realtimeSenderID(ctx)
			if err := s.realtimePublisher.PublishCardUpdated(ctx, acc.BoardID, patch, senderID); err != nil {
				return err
			}
			return s.publishRebalanced(ctx, acc.BoardID, id, rebalanced, senderID)
		})
	}
	return resp, nil
}

// fillCompletionUsers дописывает в ответ того, кто завершил, и исполнителей
// подзадачи — одним запросом пользователей на всех.
func (s *CardService) fillCompletionUsers(ctx context.Context, resp *dto.CardCompletionResponse) error {
	userIDs := append([]int64{}, resp.AssigneeIDs...)
	if resp.CompletedByID != nil {
		userIDs = append(userIDs, *resp.CompletedByID)
	}
	if len(userIDs) == 0 {
		return nil
	}
	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return err
	}
	userMap := make(map[int64]*model.User, len(users))
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}
	if resp.CompletedByID != nil {
		if u, ok := userMap[*resp.CompletedByID]; ok {
			resp.CompletedBy = &dto.CardUserResponse{
				ID:        u.ID,
				Firstname: u.Firstname,
				Lastname:  u.Lastname,
				AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
			}
		}
	}
	for _, uid := range resp.AssigneeIDs {
		if u, ok := userMap[uid]; ok {
			resp.Assignees = append(resp.Assignees, &dto.CardAssigneeResponse{
				ID:        u.ID,
				Name:      dto.UserDisplayName(*u),
				AvatarUrl: dto.UserAvatarURL(s.cfg, u.AvatarName, dto.AvatarSizeThumbnail),
			})
		}
	}
	return nil
}

// DuplicateCard копирует карточку с подзадачами наверх выбранной колонки (по
// умолчанию — той же). Что переносится, описано у запроса DuplicateCard.
// Запросов пять при любом числе подзадач: контекст с правами, целевая колонка
// с позицией, лимит доски, копия одним оператором, история. Ответ и realtime
// собираются из того, что вернула вставка.
func (s *CardService) DuplicateCard(ctx context.Context, id int64, columnID int64) (*dto.CardResponse, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, id, RoleEditor)
	if err != nil {
		return nil, err
	}
	if columnID == 0 {
		columnID = acc.ColumnID
	}
	target, err := s.repo.ColumnPrependTarget(ctx, columnID)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeColumnNotFound)
	}
	if target.ProjectID != acc.ProjectID {
		return nil, apperr.New(apperr.CodeColumnNotFound, "column not found")
	}
	activeCardsCount, err := s.repo.CountActiveCardsByBoard(ctx, target.BoardID)
	if err != nil {
		return nil, err
	}
	if activeCardsCount >= maxActiveCardsPerBoard {
		return nil, apperr.New(apperr.CodeBoardCardLimitReached, "maximum number of cards (300) on board reached")
	}

	created, children, err := s.repo.DuplicateCard(ctx, id, columnID, target.Position, currentUserID(ctx))
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:   acc.ProjectID,
		Action:      "card.duplicated",
		EntityType:  "card",
		EntityID:    created.ID,
		CardID:      created.ID,
		EntityTitle: created.Title,
		EntityLink:  historyTaskPath(acc.ProjectID, target.BoardID, created.ID),
	})

	resp := createdCardResponse(created, target.BoardID, target.Title)
	for i := range children {
		resp.Children = append(resp.Children, &dto.CardChildResponse{
			ID:       children[i].ID,
			Title:    children[i].Title,
			Position: children[i].Position,
		})
	}
	resp.ChecklistTotal = len(children)

	if s.realtimePublisher != nil {
		event := s.realtimePublisher.BuildCreatedCard(created, &model.Column{ID: columnID, BoardID: target.BoardID})
		event["checklistTotal"] = len(children)
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			return s.realtimePublisher.PublishCardCreated(ctx, target.BoardID, event, realtimeSenderID(ctx))
		})
	}
	return resp, nil
}

func currentUserID(ctx context.Context) *int64 {
	user, ok := middleware.GetUser(ctx)
	if !ok || user.ID == 0 {
		return nil
	}
	id := user.ID
	return &id
}

// filterOutActor removes the actor from the list of user IDs.
func filterOutActor(ids []int64, actor *int64) []int64 {
	if actor == nil {
		return ids
	}
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id != *actor {
			result = append(result, id)
		}
	}
	return result
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func uniqueUserIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	result := []int64{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func normalizeCardPriority(priority *string) *string {
	if priority == nil {
		return nil
	}
	value := strings.TrimSpace(*priority)
	switch value {
	case "low", "medium", "high":
		return &value
	default:
		return nil
	}
}

func normalizeCardBorderColor(color *string) *string {
	if color == nil {
		return nil
	}
	value := strings.TrimSpace(*color)
	switch value {
	case "primary", "success", "warning", "danger", "info", "dark":
		return &value
	default:
		return nil
	}
}

// publishRebalanced досылает соседям позиции, переписанные ребалансировкой
// колонки: иначе у чужих вкладок останутся старые, и следующее перетаскивание
// оттуда посчитает позицию по числам, которых в базе уже нет. Сама перемещённая
// карточка уходит патчем вместе с колонкой и статусом — её пропускаем.
// На обычном перемещении rebalanced == nil и цикл не выполняется.
//
// ponytail: по сообщению на карточку, весь цикл под общим mercurePublishTimeout.
// При большой колонке хвост может не уйти — отдельное событие с массивом позиций,
// если упрёшься (нужен новый обработчик на фронте).
func (s *CardService) publishRebalanced(ctx context.Context, boardID, movedID int64, rebalanced []model.CardPosition, senderID int64) error {
	for _, c := range rebalanced {
		if c.ID == movedID {
			continue
		}
		if err := s.realtimePublisher.PublishCardUpdated(ctx, boardID, map[string]any{
			"id":        c.ID,
			"position":  c.Position,
			"updatedAt": formatRealtimeTimeValue(c.UpdatedAt),
		}, senderID); err != nil {
			return err
		}
	}
	return nil
}

// publishParentChecklist шлёт родителю новые счётчики подзадач. Доску знает
// вызывающий — поэтому ни карточку, ни колонку ради неё не читаем.
func (s *CardService) publishParentChecklist(ctx context.Context, boardID, parentID int64) {
	if s.realtimePublisher == nil {
		return
	}
	s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
		patch, err := s.realtimePublisher.BuildChecklistCounters(ctx, parentID)
		if err != nil {
			return err
		}
		patch["id"] = parentID
		return s.realtimePublisher.PublishCardUpdated(ctx, boardID, patch, realtimeSenderID(ctx))
	})
}

func sameOptionalID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameOptionalString(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func sameOptionalTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// normalizeTimePtr brings an incoming time (from client, usually with offset) to UTC
// and truncates to seconds for consistency with TIMESTAMPTZ(0).
func normalizeTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	normalized := t.UTC().Truncate(time.Second)
	return &normalized
}
