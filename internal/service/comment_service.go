package service

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
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
	GetCommentReaders(ctx context.Context, cardID int64, commentID int64) ([]model.CommentReader, error)
}

type CommentService struct {
	repo              repository.CommentRepositoryInterface
	readRepo          repository.CommentReadRepositoryInterface
	attachmentRepo    repository.CommentAttachmentRepositoryInterface
	permSvc           *PermissionService
	userRepo          repository.UserRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
	cfg               *config.Config
	History           HistoryLogger
}

func NewCommentService(
	repo repository.CommentRepositoryInterface,
	readRepo repository.CommentReadRepositoryInterface,
	attachmentRepo repository.CommentAttachmentRepositoryInterface,
	permSvc *PermissionService,
	userRepo repository.UserRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
	notificationSvc *KanbanNotificationService,
	cfg *config.Config,
) *CommentService {
	return &CommentService{
		repo:              repo,
		readRepo:          readRepo,
		attachmentRepo:    attachmentRepo,
		permSvc:           permSvc,
		userRepo:          userRepo,
		realtimePublisher: realtimePublisher,
		notificationSvc:   notificationSvc,
		cfg:               cfg,
	}
}

// AttachFilesToComments раскладывает файлы по комментариям карточки.
// Комментарий без файлов получает пустой срез, чтобы в JSON ушёл [] вместо null.
func AttachFilesToComments(comments []model.Comment, attachments []model.CommentAttachment) {
	byID := make(map[int64]int, len(comments))
	for i := range comments {
		comments[i].Attachments = []model.CommentAttachment{}
		byID[comments[i].ID] = i
	}
	for _, a := range attachments {
		if i, ok := byID[a.CommentID]; ok {
			comments[i].Attachments = append(comments[i].Attachments, a)
		}
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
	attachments, err := s.attachmentRepo.GetByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	AttachFilesToComments(comments, attachments)
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
	if err := s.loadFiles(ctx, c); err != nil {
		return nil, err
	}
	s.populateAuthorName(ctx, c)
	return c, nil
}

func (s *CommentService) loadFiles(ctx context.Context, c *model.Comment) error {
	files, err := s.attachmentRepo.GetByComment(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Attachments = files
	return nil
}

// RequireCardComment — живой комментарий этой карточки, доступный текущему
// пользователю как viewer проекта.
func (s *CommentService) RequireCardComment(ctx context.Context, cardID, commentID int64) (*model.Comment, CardAccess, error) {
	acc, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer)
	if err != nil {
		return nil, CardAccess{}, err
	}

	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return nil, CardAccess{}, withNotFoundCode(err, apperr.CodeCommentNotFound)
	}
	if c.CardID != cardID {
		return nil, CardAccess{}, apperr.New(apperr.CodeCommentNotFound, "comment not found")
	}
	return c, acc, nil
}

// RequireOwnComment — комментарий карточки, написанный текущим пользователем.
// Права те же, что на сам комментарий: viewer проекта и авторство.
func (s *CommentService) RequireOwnComment(ctx context.Context, cardID, commentID int64) (*model.Comment, CardAccess, error) {
	c, acc, err := s.RequireCardComment(ctx, cardID, commentID)
	if err != nil {
		return nil, CardAccess{}, err
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, CardAccess{}, apperr.ErrUnauthorized
	}
	if c.AuthorID != user.ID {
		return nil, CardAccess{}, apperr.New(apperr.CodeCommentAuthorOnly, "comment author only")
	}
	return c, acc, nil
}

// PublishCommentChanged шлёт в комнату задачи комментарий целиком — после
// того, как у него добавили или сняли файл.
func (s *CommentService) PublishCommentChanged(ctx context.Context, cardID, commentID int64) {
	if s.realtimePublisher == nil {
		return
	}
	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return
	}
	if err := s.loadFiles(ctx, c); err != nil {
		return
	}
	s.populateAuthorName(ctx, c)
	s.publishComment(ctx, cardID, map[string]any{
		"type":    "comment_updated",
		"comment": dto.MapCommentResponse(s.cfg, c),
	})
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
		Body:        body,
		CardID:      cardID,
		AuthorID:    user.ID,
		Attachments: []model.CommentAttachment{},
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
	s.publishCommentsCount(ctx, acc.BoardID, cardID)
	s.publishComment(ctx, cardID, map[string]any{
		"type":    "comment_created",
		"comment": dto.MapCommentResponse(s.cfg, created),
	})

	// Comment notification (unified)
	if s.notificationSvc != nil {
		actorID := derefInt64(currentUserID(ctx))
		runDetached(ctx, notifyTimeout, "failed to notify kanban comment added", func(ctx context.Context) error {
			// Проект, доска, заголовок и автор уже известны — уведомление
			// не перечитывает ради них карточку, доску и пользователя.
			s.notificationSvc.NotifyCommentAdded(ctx, acc.ProjectID, acc.BoardID, acc.BoardTitle, cardID, actorID, acc.CardTitle, created.AuthorName)
			return nil
		})
	}
	return created, nil
}

func (s *CommentService) UpdateComment(ctx context.Context, cardID int64, commentID int64, req dto.UpdateCommentRequest) (*model.Comment, error) {
	c, acc, err := s.RequireOwnComment(ctx, cardID, commentID)
	if err != nil {
		return nil, err
	}

	if req.Body == nil {
		return nil, apperr.New(apperr.CodeCommentBodyRequired, "comment body required")
	}
	oldBody := c.Body
	body, err := normalizeCommentBody(*req.Body)
	if err != nil {
		return nil, err
	}
	if err := s.loadFiles(ctx, c); err != nil {
		return nil, err
	}
	// Без текста комментарий живёт только на файлах.
	if body == "" && len(c.Attachments) == 0 {
		return nil, apperr.New(apperr.CodeCommentBodyRequired, "comment body required")
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
	s.publishComment(ctx, cardID, map[string]any{
		"type":    "comment_updated",
		"comment": dto.MapCommentResponse(s.cfg, updated),
	})
	return updated, nil
}

func (s *CommentService) DeleteComment(ctx context.Context, cardID int64, commentID int64) error {
	_, acc, err := s.RequireOwnComment(ctx, cardID, commentID)
	if err != nil {
		return err
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
	s.publishCommentsCount(ctx, acc.BoardID, cardID)
	s.publishComment(ctx, cardID, map[string]any{
		"type":      "comment_deleted",
		"commentId": commentID,
	})
	return nil
}

// publishComment шлёт текст комментария в комнату задачи. Счётчик на доске
// по-прежнему уходит отдельно, его слушают и те, у кого задача не открыта.
func (s *CommentService) publishComment(ctx context.Context, cardID int64, event map[string]any) {
	if s.realtimePublisher == nil {
		return
	}
	event["cardId"] = cardID
	event["senderId"] = realtimeSenderID(ctx)
	s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
		return s.realtimePublisher.PublishCardComment(ctx, cardID, event)
	})
}

// publishCommentsCount шлёт доске новый счётчик комментариев. Доску знает
// вызывающий — карточку и колонку ради неё не читаем.
func (s *CommentService) publishCommentsCount(ctx context.Context, boardID, cardID int64) {
	if s.realtimePublisher == nil {
		return
	}
	s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
		patch, err := s.realtimePublisher.BuildCommentsCount(ctx, cardID)
		if err != nil {
			return err
		}
		patch["id"] = cardID
		return s.realtimePublisher.PublishCardUpdated(ctx, boardID, patch, realtimeSenderID(ctx))
	})
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

// GetCommentReaders — содержимое модалки «кто видел». Список строится от
// отметок, а не от текущего состава проекта: выбывший участник, который
// комментарий прочитал, из выдачи не пропадает.
func (s *CommentService) GetCommentReaders(ctx context.Context, cardID int64, commentID int64) ([]model.CommentReader, error) {
	if _, err := s.permSvc.RequireRootCardRole(ctx, cardID, RoleViewer); err != nil {
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

	// Автор выпадает: «Иванов прочитал комментарий Иванова» — шум, а отметка
	// ему проставляется автоматически при создании.
	readAt := make(map[int64]time.Time, len(marks))
	userIDs := make([]int64, 0, len(marks))
	for _, m := range marks {
		if m.UserID == c.AuthorID {
			continue
		}
		readAt[m.UserID] = m.ReadAt
		userIDs = append(userIDs, m.UserID)
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	return buildCommentReaders(users, readAt, c.UpdatedAt), nil
}

// buildCommentReaders сшивает отметки с людьми и сортирует по времени.
// updatedAt — время правки комментария, если она была.
func buildCommentReaders(users []model.User, readAt map[int64]time.Time, updatedAt *time.Time) []model.CommentReader {
	readers := make([]model.CommentReader, 0, len(users))
	for i := range users {
		t, ok := readAt[users[i].ID]
		if !ok {
			continue
		}
		readers = append(readers, model.CommentReader{
			User:   users[i],
			ReadAt: t,
			// Текст правили после того, как человек его прочитал: в споре это
			// значит, что видел он другую формулировку.
			ReadBeforeEdit: updatedAt != nil && updatedAt.After(t),
		})
	}

	sort.Slice(readers, func(i, j int) bool {
		return readers[i].ReadAt.Before(readers[j].ReadAt)
	})
	return readers
}

// normalizeCommentBody режет пробелы и длину; пустоту решает вызывающий —
// комментарий может держаться на одних файлах.
func normalizeCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
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
