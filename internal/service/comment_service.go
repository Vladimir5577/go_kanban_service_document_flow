package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const (
	maxCommentBodyLength = 10000
	maxCommentsPerCard   = 300
)

type CommentServiceInterface interface {
	GetComments(ctx context.Context, cardID int64) ([]model.Comment, error)
	GetComment(ctx context.Context, commentID int64) (*model.Comment, error)
	CreateComment(ctx context.Context, cardID int64, req dto.CreateCommentRequest) (*model.Comment, error)
	UpdateComment(ctx context.Context, cardID int64, commentID int64, req dto.UpdateCommentRequest) (*model.Comment, error)
	DeleteComment(ctx context.Context, cardID int64, commentID int64) error
	MarkRead(ctx context.Context, cardID int64, lastCommentID int64) error
	GetCommentReaders(ctx context.Context, cardID int64, commentID int64) (*model.CommentReaders, error)
}

type CommentService struct {
	repo              repository.CommentRepositoryInterface
	readRepo          repository.CommentReadRepositoryInterface
	memberRepo        repository.ProjectMemberRepositoryInterface
	permSvc           *PermissionService
	userRepo          repository.UserRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
	History           HistoryLogger
}

func NewCommentService(
	repo repository.CommentRepositoryInterface,
	readRepo repository.CommentReadRepositoryInterface,
	memberRepo repository.ProjectMemberRepositoryInterface,
	permSvc *PermissionService,
	userRepo repository.UserRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
	notificationSvc *KanbanNotificationService,
) *CommentService {
	return &CommentService{
		repo:              repo,
		readRepo:          readRepo,
		memberRepo:        memberRepo,
		permSvc:           permSvc,
		userRepo:          userRepo,
		realtimePublisher: realtimePublisher,
		notificationSvc:   notificationSvc,
	}
}

func (s *CommentService) GetComments(ctx context.Context, cardID int64) ([]model.Comment, error) {
	if _, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer); err != nil {
		return nil, err
	}
	comments, err := s.repo.GetComments(ctx, cardID)
	if err != nil {
		return nil, err
	}
	s.populateAuthorNames(ctx, comments)
	return comments, nil
}

func (s *CommentService) GetComment(ctx context.Context, commentID int64) (*model.Comment, error) {
	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCommentNotFound)
	}
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, c.CardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	s.populateAuthorName(ctx, c)
	return c, nil
}

func (s *CommentService) CreateComment(ctx context.Context, cardID int64, req dto.CreateCommentRequest) (*model.Comment, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return nil, err
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrUnauthorized
	}

	body, err := normalizeCommentBody(req.Body)
	if err != nil {
		return nil, err
	}

	// Лимит проверяется счётчиком: раньше ради len() поднимались все комментарии
	// карточки с телами. Фильтр у обоих запросов одинаковый — deleted_at IS NULL.
	counts, err := s.repo.GetCountsByCardIDs(ctx, []int64{cardID})
	if err != nil {
		return nil, err
	}
	if counts[cardID] >= maxCommentsPerCard {
		return nil, apperr.New(apperr.CodeCommentLimitReached, "maximum number of comments (300) per card reached")
	}

	c := &model.Comment{
		Body:     body,
		CardID:   cardID,
		AuthorID: user.ID,
	}
	created, err := s.repo.CreateComment(ctx, cardID, c)
	if err != nil {
		return nil, err
	}
	s.populateAuthorName(ctx, created)
	// Свой комментарий не должен висеть у автора непрочитанным — иначе он сам
	// себе создаёт бейдж. Не повод валить создание комментария, если не легло.
	if err := s.readRepo.MarkRead(ctx, cardID, user.ID, created.ID); err != nil {
		slog.Warn("failed to mark own comment read", "card_id", cardID, "comment_id", created.ID, "error", err)
	}
	// Ссылка собирается из acc — иначе historyEntityLink пойдёт добывать проект
	// и доску заново, а это GetCard (карточка + assignees + labels) и GetColumn.
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:  acc.ProjectID,
		Action:     "comment.created",
		EntityType: "comment",
		EntityID:   created.ID,
		CardID:     cardID,
		EntityLink: historyTaskPath(acc.ProjectID, acc.BoardID, cardID),
	})
	if s.realtimePublisher != nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			patch, err := s.realtimePublisher.BuildCommentsCount(ctx, cardID)
			if err != nil {
				return err
			}
			return s.realtimePublisher.PublishCardPatchByID(ctx, cardID, patch, realtimeSenderID(ctx))
		})
	}

	// Comment notification (unified)
	if s.notificationSvc != nil {
		actorID := derefInt64(currentUserID(ctx))
		runDetached(ctx, notifyTimeout, "failed to notify kanban comment added", func(ctx context.Context) error {
			// Проект и доска уже в acc: без них resolveBoardID внутри уведомления
			// снова читал бы карточку с колонкой.
			s.notificationSvc.NotifyCommentAdded(ctx, acc.ProjectID, acc.BoardID, cardID, actorID, "")
			return nil
		})
	}
	return created, nil
}

func (s *CommentService) UpdateComment(ctx context.Context, cardID int64, commentID int64, req dto.UpdateCommentRequest) (*model.Comment, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return nil, err
	}

	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCommentNotFound)
	}
	if c.CardID != cardID {
		return nil, apperr.New(apperr.CodeCommentNotFound, "comment not found")
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrUnauthorized
	}
	if c.AuthorID != user.ID {
		return nil, apperr.New(apperr.CodeCommentAuthorOnly, "comment author only")
	}

	if req.Body == nil {
		return nil, apperr.New(apperr.CodeCommentBodyRequired, "comment body required")
	}
	oldBody := c.Body
	body, err := normalizeCommentBody(*req.Body)
	if err != nil {
		return nil, err
	}
	c.Body = body

	updated, err := s.repo.UpdateComment(ctx, c)
	if err != nil {
		return nil, err
	}
	s.populateAuthorName(ctx, updated)
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:  acc.ProjectID,
		Action:     "comment.updated",
		EntityType: "comment",
		EntityID:   commentID,
		CardID:     cardID,
		EntityLink: historyTaskPath(acc.ProjectID, acc.BoardID, cardID),
		Before:     oldBody,
		After:      body,
	})
	return updated, nil
}

func (s *CommentService) DeleteComment(ctx context.Context, cardID int64, commentID int64) error {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return err
	}

	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return withNotFoundCode(err, apperr.CodeCommentNotFound)
	}
	if c.CardID != cardID {
		return apperr.New(apperr.CodeCommentNotFound, "comment not found")
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return apperr.ErrUnauthorized
	}
	if c.AuthorID != user.ID {
		return apperr.New(apperr.CodeCommentAuthorOnly, "comment author only")
	}

	if err := s.repo.DeleteComment(ctx, commentID); err != nil {
		return err
	}
	appendHistory(s.History, ctx, model.HistoryWrite{
		ProjectID:  acc.ProjectID,
		Action:     "comment.deleted",
		EntityType: "comment",
		EntityID:   commentID,
		CardID:     cardID,
		EntityLink: historyTaskPath(acc.ProjectID, acc.BoardID, cardID),
	})
	if s.realtimePublisher != nil {
		s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
			patch, err := s.realtimePublisher.BuildCommentsCount(ctx, cardID)
			if err != nil {
				return err
			}
			return s.realtimePublisher.PublishCardPatchByID(ctx, cardID, patch, realtimeSenderID(ctx))
		})
	}
	return nil
}

// MarkRead двигает знак прочтения карточки. Что именно считать прочитанным,
// решает фронт — сюда приходит одно число, и оно может только расти.
func (s *CommentService) MarkRead(ctx context.Context, cardID int64, lastCommentID int64) error {
	if _, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer); err != nil {
		return err
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return apperr.ErrUnauthorized
	}

	return s.readRepo.MarkRead(ctx, cardID, user.ID, lastCommentID)
}

// GetCommentReaders — содержимое модалки «кто видел». Автор не попадает ни в
// один из списков: свой комментарий он видел по определению.
func (s *CommentService) GetCommentReaders(ctx context.Context, cardID int64, commentID int64) (*model.CommentReaders, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return nil, err
	}

	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return nil, withNotFoundCode(err, apperr.CodeCommentNotFound)
	}
	if c.CardID != cardID {
		return nil, apperr.New(apperr.CodeCommentNotFound, "comment not found")
	}

	marks, err := s.readRepo.GetReaders(ctx, cardID, commentID)
	if err != nil {
		return nil, err
	}
	readAt := make(map[int64]time.Time, len(marks))
	for _, m := range marks {
		readAt[m.UserID] = m.ReadAt
	}

	members, err := s.memberRepo.GetMembers(ctx, acc.ProjectID)
	if err != nil {
		return nil, err
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, commentAudienceIDs(acc.OwnerID, members, c.AuthorID))
	if err != nil {
		return nil, err
	}

	return splitCommentReaders(users, readAt, c.UpdatedAt), nil
}

// commentAudienceIDs — кого вообще показывать в модалке. Владелец проекта
// строки в kanban_project_user не имеет, поэтому идёт отдельно; автор
// исключается — свой комментарий он видел по определению.
func commentAudienceIDs(ownerID int64, members []model.ProjectUser, authorID int64) []int64 {
	audience := make([]int64, 0, len(members)+1)
	seen := map[int64]bool{authorID: true}
	add := func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		audience = append(audience, id)
	}
	add(ownerID)
	for i := range members {
		add(members[i].UserID)
	}
	return audience
}

// splitCommentReaders раскладывает участников на прочитавших и остальных.
// updatedAt — время правки комментария, если она была.
func splitCommentReaders(users []model.User, readAt map[int64]time.Time, updatedAt *time.Time) *model.CommentReaders {
	res := &model.CommentReaders{
		Readers: make([]model.CommentReader, 0, len(users)),
		Pending: make([]model.User, 0, len(users)),
	}
	for i := range users {
		t, ok := readAt[users[i].ID]
		if !ok {
			res.Pending = append(res.Pending, users[i])
			continue
		}
		res.Readers = append(res.Readers, model.CommentReader{
			User:   users[i],
			ReadAt: t,
			// Текст правили после того, как человек его прочитал: в споре это
			// значит, что видел он другую формулировку.
			ReadBeforeEdit: updatedAt != nil && updatedAt.After(t),
		})
	}

	sort.Slice(res.Readers, func(i, j int) bool {
		return res.Readers[i].ReadAt.Before(res.Readers[j].ReadAt)
	})
	sort.Slice(res.Pending, func(i, j int) bool {
		return dto.UserDisplayName(res.Pending[i]) < dto.UserDisplayName(res.Pending[j])
	})
	return res
}

func normalizeCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", apperr.New(apperr.CodeCommentBodyRequired, "comment body required")
	}
	if utf8.RuneCountInString(body) > maxCommentBodyLength {
		return "", apperr.New(apperr.CodeCommentBodyTooLong, "comment body too long")
	}
	return body, nil
}

func (s *CommentService) populateAuthorName(ctx context.Context, comment *model.Comment) {
	if comment == nil {
		return
	}
	users, err := s.userRepo.GetUsersByIDs(ctx, []int64{comment.AuthorID})
	if err != nil || len(users) == 0 {
		return
	}
	comment.AuthorName = commentAuthorName(&users[0])
}

func (s *CommentService) populateAuthorNames(ctx context.Context, comments []model.Comment) {
	if len(comments) == 0 {
		return
	}
	// Авторов обычно единицы на сотню комментариев — шлём каждого по разу.
	seen := make(map[int64]bool, len(comments))
	userIDs := make([]int64, 0, len(comments))
	for i := range comments {
		if id := comments[i].AuthorID; !seen[id] {
			seen[id] = true
			userIDs = append(userIDs, id)
		}
	}
	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return
	}
	userMap := make(map[int64]*model.User)
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}
	for i := range comments {
		if u, ok := userMap[comments[i].AuthorID]; ok {
			comments[i].AuthorName = commentAuthorName(u)
		}
	}
}

func commentAuthorName(u *model.User) string {
	return dto.UserDisplayName(*u)
}
