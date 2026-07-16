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
	CreateCard(ctx context.Context, req dto.CreateCardRequest) (*model.Card, error)
	GetCard(ctx context.Context, id int64) (*model.Card, error)
	GetCardDetail(ctx context.Context, id int64) (*dto.CardResponse, error)
	GetAssignedToMe(ctx context.Context, status string) (*dto.AssignedToMeResponse, error)
	UpdateCard(ctx context.Context, id int64, req dto.UpdateCardRequest) (*model.Card, error)
	DeleteCard(ctx context.Context, id int64) error
	UpdateAssignees(ctx context.Context, id int64, userIDs []int64) error
	MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.Card, error)
	ArchiveCard(ctx context.Context, id int64) error
	CompleteCard(ctx context.Context, id int64) (*model.Card, error)
}

type CardService struct {
	repo              repository.CardRepositoryInterface
	permSvc           *PermissionService
	minioSvc          MinioServiceInterface
	subtaskRepo       repository.SubtaskRepositoryInterface
	commentRepo       repository.CommentRepositoryInterface
	attachmentRepo    repository.AttachmentRepositoryInterface
	labelRepo         repository.LabelRepositoryInterface
	userRepo          repository.UserRepositoryInterface
	activityRepo      repository.ActivityRepositoryInterface
	columnRepo        repository.ColumnRepositoryInterface
	projectRepo       repository.ProjectRepositoryInterface
	projectMemberRepo repository.ProjectMemberRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
	cfg               *config.Config
}

const maxActiveCardsPerBoard = 300

func NewCardService(
	repo repository.CardRepositoryInterface,
	permSvc *PermissionService,
	minioSvc MinioServiceInterface,
	subtaskRepo repository.SubtaskRepositoryInterface,
	commentRepo repository.CommentRepositoryInterface,
	attachmentRepo repository.AttachmentRepositoryInterface,
	labelRepo repository.LabelRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	activityRepo repository.ActivityRepositoryInterface,
	columnRepo repository.ColumnRepositoryInterface,
	projectRepo repository.ProjectRepositoryInterface,
	projectMemberRepo repository.ProjectMemberRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
	notificationSvc *KanbanNotificationService,
	cfg *config.Config,
) *CardService {
	return &CardService{
		repo:              repo,
		permSvc:           permSvc,
		minioSvc:          minioSvc,
		subtaskRepo:       subtaskRepo,
		commentRepo:       commentRepo,
		attachmentRepo:    attachmentRepo,
		labelRepo:         labelRepo,
		userRepo:          userRepo,
		activityRepo:      activityRepo,
		columnRepo:        columnRepo,
		projectRepo:       projectRepo,
		projectMemberRepo: projectMemberRepo,
		realtimePublisher: realtimePublisher,
		notificationSvc:   notificationSvc,
		cfg:               cfg,
	}
}

func (s *CardService) CreateCard(ctx context.Context, req dto.CreateCardRequest) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByColumn(ctx, req.ColumnID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}
	column, err := s.columnRepo.GetColumn(ctx, req.ColumnID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeColumnNotFound)
	}

	activeCardsCount, err := s.repo.CountActiveCardsByBoard(ctx, column.BoardID)
	if err != nil {
		return nil, err
	}
	if activeCardsCount >= maxActiveCardsPerBoard {
		return nil, apperr.New(apperr.CodeBoardCardLimitReached, "maximum number of cards (300) on board reached")
	}

	if len(req.AssigneeIDs) > 1 {
		return nil, apperr.New(apperr.CodeValidation, "maximum 1 assignee allowed")
	}

	c := &model.Card{
		Title:       req.Title,
		ColumnID:    req.ColumnID,
		Description: req.Description,
		DueDate:     normalizeTimePtr(req.DueDate),
		Priority:    req.Priority,
		BorderColor: req.BorderColor,
		AssigneeIDs: req.AssigneeIDs,
		LabelIDs:    req.LabelIDs,
	}
	if authorID := currentUserID(ctx); authorID != nil {
		c.CreatedByID = authorID
	}
	if req.Position != nil {
		c.Position = *req.Position
	} else {
		cards, _ := s.repo.GetCardsByColumn(ctx, req.ColumnID)
		if len(cards) > 0 {
			// Prepend new cards at the top of the column (smallest position first)
			c.Position = cards[0].Position - 65536.0
		} else {
			c.Position = 65536.0
		}
	}
	created, err := s.repo.CreateCard(ctx, req.ColumnID, c)
	if err == nil && created != nil {
		s.logActivity(ctx, created.ID, "created", nil, nil)
		if s.realtimePublisher != nil {
			s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
				return s.realtimePublisher.PublishCardCreated(
					ctx,
					column.BoardID,
					s.realtimePublisher.BuildCreatedCard(created, column),
					realtimeSenderID(ctx),
				)
			})
		}

		// Notifications via unified service
		projectID, _ := s.permSvc.GetProjectIDByColumn(ctx, req.ColumnID)
		actorID := currentUserID(ctx)
		if s.notificationSvc != nil {
			s.notificationSvc.NotifyCardCreated(ctx, projectID, column.BoardID, created.ID, derefInt64(actorID), created.Title)

			for _, aid := range created.AssigneeIDs {
				s.notificationSvc.NotifyTaskAssigned(ctx, projectID, column.BoardID, created.ID, derefInt64(actorID), aid, created.Title, false)
			}
		}
	}
	return created, err
}

func (s *CardService) GetCardDetail(ctx context.Context, id int64) (*dto.CardResponse, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	card, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	resp := dto.MapCardResponse(card)

	// Fetch Subtasks
	subtasks, err := s.subtaskRepo.GetSubtasks(ctx, id)
	if err != nil {
		return nil, err
	}
	resp.Subtasks = dto.MapSubtasksResponse(subtasks)
	for _, st := range subtasks {
		resp.ChecklistTotal++
		if st.Status == "done" {
			resp.ChecklistDone++
		}
	}

	// Fetch Comments
	comments, err := s.commentRepo.GetComments(ctx, id)
	if err != nil {
		return nil, err
	}

	var userIDs []int64
	for i := range comments {
		userIDs = append(userIDs, comments[i].AuthorID)
	}
	for _, aid := range card.AssigneeIDs {
		userIDs = append(userIDs, aid)
	}
	for i := range resp.Subtasks {
		if resp.Subtasks[i].UserID != nil {
			userIDs = append(userIDs, *resp.Subtasks[i].UserID)
		}
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
		return nil, err
	}
	allAttachments = append(allAttachments, attsCard...)

	attsDesc, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "description")
	if err != nil {
		return nil, err
	}
	allAttachments = append(allAttachments, attsDesc...)

	chatAttachments, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "chat")
	if err != nil {
		return nil, err
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
		return nil, err
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

	resp.Attachments = dto.MapAttachmentsResponse(s.cfg, allAttachments)
	for i, att := range resp.Attachments {
		if att.AuthorID != nil {
			if u, ok := userMap[*att.AuthorID]; ok {
				name := dto.UserDisplayName(*u)
				resp.Attachments[i].AuthorName = &name
			}
		}
	}

	for _, st := range resp.Subtasks {
		if st.UserID != nil {
			if u, ok := userMap[*st.UserID]; ok {
				name := dto.UserDisplayName(*u)
				st.UserName = &name
			}
		}
	}

	col, err := s.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil {
		return nil, err
	}

	// Enrich with board and column info required by frontend
	resp.BoardID = col.BoardID
	resp.ColumnTitle = col.Title

	labels, err := s.labelRepo.GetLabels(ctx, col.BoardID)
	if err != nil {
		return nil, err
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

	return resp, nil
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

func (s *CardService) GetAssignedToMe(ctx context.Context, status string) (*dto.AssignedToMeResponse, error) {
	userID := currentUserID(ctx)
	if userID == nil {
		return nil, apperr.ErrUnauthorized
	}

	cardRows, err := s.repo.GetAssignedCards(ctx, *userID, status)
	if err != nil {
		return nil, err
	}
	subtaskRows, err := s.repo.GetAssignedSubtasks(ctx, *userID, status)
	if err != nil {
		return nil, err
	}

	return &dto.AssignedToMeResponse{
		AssignedCards:    buildAssignedTree(cardRows),
		AssignedSubtasks: mapAssignedSubtasks(subtaskRows),
	}, nil
}

func (s *CardService) UpdateCard(ctx context.Context, id int64, req dto.UpdateCardRequest) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	c, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	var titleChanged, descChanged, dueChanged, priorityChanged, colorChanged bool
	var oldTitle, newTitle *string
	var oldDue, newDue *string
	var oldPriority, newPriority *string
	var oldColor, newColor *string

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
		oldDescription := c.Description
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
			oldPriorityValue := formatPriority(c.Priority)
			newPriorityValue := formatPriority(normalizedPriority)
			oldPriority, newPriority = &oldPriorityValue, &newPriorityValue
			c.Priority = normalizedPriority
		}
	}

	if req.HasBorderColor {
		normalizedColor := normalizeCardBorderColor(req.BorderColor)
		if !sameOptionalString(c.BorderColor, normalizedColor) {
			colorChanged = true
			oldColorValue := valueOr(c.BorderColor, "без цвета")
			newColorValue := valueOr(normalizedColor, "без цвета")
			oldColor, newColor = &oldColorValue, &newColorValue
			c.BorderColor = normalizedColor
		}
	}

	if !titleChanged && !descChanged && !dueChanged && !priorityChanged && !colorChanged {
		return c, nil
	}

	updatedCard, err := s.repo.UpdateCard(ctx, c)
	if err == nil {
		if titleChanged {
			s.logActivity(ctx, id, "renamed", oldTitle, newTitle)
		}
		if descChanged {
			s.logActivity(ctx, id, "description_changed", nil, nil)
		}
		if priorityChanged {
			s.logActivity(ctx, id, "priority_changed", oldPriority, newPriority)
		}
		if dueChanged {
			if oldDue == nil {
				none := "не задан"
				oldDue = &none
			}
			if newDue == nil {
				none := "не задан"
				newDue = &none
			}
			s.logActivity(ctx, id, "due_date_changed", oldDue, newDue)
		}
		if colorChanged {
			s.logActivity(ctx, id, "color_changed", oldColor, newColor)
		}
		if s.realtimePublisher != nil {
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
				s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
					return s.realtimePublisher.PublishCardPatch(ctx, updatedCard, patch, realtimeSenderID(ctx))
				})
			}
		}
	}
	return updatedCard, err
}

func (s *CardService) DeleteCard(ctx context.Context, id int64) error {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}
	card, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	column, err := s.columnRepo.GetColumn(ctx, card.ColumnID)
	if err != nil {
		return withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeColumnNotFound)
	}

	// Получаем все вложения для удаления файлов из MinIO
	attachments, err := s.attachmentRepo.GetAttachmentsByCard(ctx, id, "")
	if err == nil && len(attachments) > 0 {
		for _, att := range attachments {
			_ = s.minioSvc.DeleteObject(ctx, s.cfg.MinioBucket, att.StorageKey)
			_ = s.attachmentRepo.DeleteAttachment(ctx, att.ID)
		}
	}

	err = s.repo.DeleteCard(ctx, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperr.New(apperr.CodeValidation, "Нельзя удалить задачу, пока в ней есть прикрепленные данные.")
		}
		return err
	}
	if s.realtimePublisher != nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			return s.realtimePublisher.PublishCardDeleted(ctx, column.BoardID, id, realtimeSenderID(ctx))
		})
	}
	return nil
}

func (s *CardService) UpdateAssignees(ctx context.Context, id int64, userIDs []int64) error {
	if len(userIDs) > 1 {
		return apperr.New(apperr.CodeValidation, "maximum 1 assignee allowed")
	}

	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}
	if err := s.validateProjectAssignees(ctx, projectID, userIDs); err != nil {
		return err
	}
	var oldValue *string
	card, _ := s.repo.GetCard(ctx, id)
	if card != nil && len(card.AssigneeIDs) > 0 {
		if users, _ := s.userRepo.GetUsersByIDs(ctx, []int64{card.AssigneeIDs[0]}); len(users) > 0 {
			n := dto.UserDisplayName(users[0])
			oldValue = &n
		}
	}

	var newValue *string
	if len(userIDs) > 0 {
		if users, _ := s.userRepo.GetUsersByIDs(ctx, []int64{userIDs[0]}); len(users) > 0 {
			n := dto.UserDisplayName(users[0])
			newValue = &n
		}
	}

	err = s.repo.UpdateCardAssignees(ctx, id, userIDs)
	if err == nil {
		if oldValue != nil && newValue == nil {
			s.logActivity(ctx, id, "assignee_removed", oldValue, nil)
		} else if oldValue == nil && newValue != nil {
			s.logActivity(ctx, id, "assignee_added", nil, newValue)
		} else if oldValue != nil && newValue != nil && *oldValue != *newValue {
			s.logActivity(ctx, id, "assignee_removed", oldValue, nil)
			s.logActivity(ctx, id, "assignee_added", nil, newValue)
		}
		if s.realtimePublisher != nil {
			s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
				patch, err := s.realtimePublisher.BuildAssignees(ctx, id)
				if err != nil {
					return err
				}
				return s.realtimePublisher.PublishCardPatchByID(ctx, id, patch, realtimeSenderID(ctx))
			})
		}

		// Notify assignee
		if s.notificationSvc != nil && len(userIDs) > 0 {
			projectID, _ := s.permSvc.GetProjectIDByCard(ctx, id)
			card, _ := s.repo.GetCard(ctx, id)
			actorID := currentUserID(ctx)
			newAssignee := userIDs[0]
			// board may be resolved inside if not passed; here we don't have column loaded cheaply
			s.notificationSvc.NotifyTaskAssigned(ctx, projectID, 0, id, derefInt64(actorID), newAssignee, card.Title, false)
		}
	}
	return err
}

func (s *CardService) validateProjectAssignees(ctx context.Context, projectID int64, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return err
	}
	if len(users) != len(userIDs) {
		return apperr.New(apperr.CodeUserNotFound, "user not found")
	}

	project, err := s.projectRepo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		if userID == project.OwnerID {
			continue
		}
		if _, err := s.projectMemberRepo.GetProjectMember(ctx, projectID, userID); err != nil {
			if errors.Is(err, apperr.ErrNotFound) {
				return apperr.New(apperr.CodeUserNotProjectMember, "user is not project member")
			}
			return err
		}
	}
	return nil
}

func (s *CardService) MoveCard(ctx context.Context, id int64, columnID int64, position float64) (*model.Card, error) {
	if columnID == 0 {
		return nil, apperr.New(apperr.CodeColumnIDAndPositionRequired, "column_id and position required")
	}

	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	cardBefore, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}
	sourceColumn, err := s.columnRepo.GetColumn(ctx, cardBefore.ColumnID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeColumnNotFound)
	}
	targetColumn, err := s.columnRepo.GetColumn(ctx, columnID)
	if err != nil {
		return nil, withNotFoundCode(mapNoRowsToNotFound(err), apperr.CodeColumnNotFound)
	}
	if targetColumn.BoardID != sourceColumn.BoardID {
		return nil, apperr.New(apperr.CodeColumnNotFound, "column not found")
	}

	columnChanged := cardBefore.ColumnID != columnID

	var oldValue *string
	if columnChanged {
		oldValue = &sourceColumn.Title
	}

	var newValue *string
	if columnChanged {
		newValue = &targetColumn.Title
	}

	card, err := s.repo.MoveCard(ctx, id, columnID, position)
	if err == nil && columnChanged {
		s.logActivity(ctx, id, "moved", oldValue, newValue)
	}
	if err == nil && s.realtimePublisher != nil {
		patch := map[string]any{
			"id":        card.ID,
			"position":  card.Position,
			"updatedAt": formatRealtimeTimeValue(card.UpdatedAt),
		}
		if columnChanged {
			patch["columnId"] = targetColumn.ID
			patch["columnTitle"] = targetColumn.Title
			patch["status"] = strconv.FormatInt(targetColumn.ID, 10)
		}
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			return s.realtimePublisher.PublishCardUpdated(ctx, targetColumn.BoardID, patch, realtimeSenderID(ctx))
		})
	}

	// Notify on column change (moved)
	if columnChanged && s.notificationSvc != nil {
		projectID, _ := s.permSvc.GetProjectIDByCard(ctx, id)
		actorID := currentUserID(ctx)
		// source and target are guaranteed to be on the same board
		s.notificationSvc.NotifyTaskMoved(ctx, projectID, sourceColumn.BoardID, id, derefInt64(actorID), card.Title, sourceColumn.Title, targetColumn.Title)
	}

	return card, err
}

func (s *CardService) ArchiveCard(ctx context.Context, id int64) error {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}

	card, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	activityType := "archived"
	if card.IsArchived {
		activityType = "restored"
		card.IsArchived = false
		card.ArchivedAt = nil
		card.ArchivedByID = nil
	} else {
		card.IsArchived = true
		now := s.cfg.Clock.Now()
		card.ArchivedAt = &now
		card.ArchivedByID = currentUserID(ctx)
	}

	_, err = s.repo.UpdateCard(ctx, card)
	if err == nil {
		s.logActivity(ctx, id, activityType, nil, nil)
	}
	return err
}

func (s *CardService) CompleteCard(ctx context.Context, id int64) (*model.Card, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	card, err := s.repo.GetCard(ctx, id)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCardNotFound)
	}

	activityType := "completed"
	if card.CompletedAt != nil {
		activityType = "reopened"
		card.CompletedAt = nil
		card.CompletedByID = nil
	} else {
		now := s.cfg.Clock.Now()
		card.CompletedAt = &now
		card.CompletedByID = currentUserID(ctx)
	}

	updated, err := s.repo.UpdateCard(ctx, card)
	if err == nil {
		s.logActivity(ctx, id, activityType, nil, nil)
		if s.realtimePublisher != nil {
			s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
				return s.realtimePublisher.PublishCardPatch(ctx, updated, map[string]any{
					"completedAt":   formatRealtimeTime(updated.CompletedAt),
					"completedById": updated.CompletedByID,
				}, realtimeSenderID(ctx))
			})
		}
	}
	return updated, err
}

func (s *CardService) logActivity(ctx context.Context, cardID int64, action string, oldValue, newValue *string) {
	_ = s.activityRepo.LogActivity(ctx, cardID, currentUserID(ctx), action, oldValue, newValue)
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

func valueOr(v *string, fallback string) string {
	if v == nil || *v == "" {
		return fallback
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

func formatPriority(priority *string) string {
	if priority == nil || *priority == "" {
		return "не задан"
	}
	switch *priority {
	case "low":
		return "Низкий"
	case "medium":
		return "Средний"
	case "high":
		return "Высокий"
	default:
		return *priority
	}
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
