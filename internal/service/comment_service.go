package service

import (
	"context"
	"strings"
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
}

type CommentService struct {
	repo              repository.CommentRepositoryInterface
	permSvc           *PermissionService
	userRepo          repository.UserRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
}

func NewCommentService(
	repo repository.CommentRepositoryInterface,
	permSvc *PermissionService,
	userRepo repository.UserRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
	notificationSvc *KanbanNotificationService,
) *CommentService {
	return &CommentService{
		repo:              repo,
		permSvc:           permSvc,
		userRepo:          userRepo,
		realtimePublisher: realtimePublisher,
		notificationSvc:   notificationSvc,
	}
}

func (s *CommentService) GetComments(ctx context.Context, cardID int64) ([]model.Comment, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
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
		return nil, err
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
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
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

	comments, err := s.repo.GetComments(ctx, cardID)
	if err == nil && len(comments) >= maxCommentsPerCard {
		return nil, apperr.New(apperr.CodeConflict, "maximum number of comments (300) per card reached")
	}
	if err != nil {
		return nil, err
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
		projectID, _ := s.permSvc.GetProjectIDByCard(ctx, cardID)
		actorID := currentUserID(ctx)
		// boardID may be resolved inside the notification service via the card
		s.notificationSvc.NotifyCommentAdded(ctx, projectID, 0, cardID, derefInt64(actorID), "")
	}
	return created, nil
}

func (s *CommentService) UpdateComment(ctx context.Context, cardID int64, commentID int64, req dto.UpdateCommentRequest) (*model.Comment, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}

	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if c.CardID != cardID {
		return nil, apperr.ErrNotFound
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrUnauthorized
	}
	if c.AuthorID != user.ID {
		return nil, apperr.ErrForbidden
	}

	if req.Body == nil {
		return nil, apperr.New(apperr.CodeValidation, "comment body required")
	}
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
	return updated, nil
}

func (s *CommentService) DeleteComment(ctx context.Context, cardID int64, commentID int64) error {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return err
	}

	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return err
	}
	if c.CardID != cardID {
		return apperr.ErrNotFound
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return apperr.ErrUnauthorized
	}
	if c.AuthorID != user.ID {
		return apperr.ErrForbidden
	}

	if err := s.repo.DeleteComment(ctx, commentID); err != nil {
		return err
	}
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

func normalizeCommentBody(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", apperr.New(apperr.CodeValidation, "comment body required")
	}
	if utf8.RuneCountInString(body) > maxCommentBodyLength {
		return "", apperr.New(apperr.CodeValidation, "comment body too long")
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
	var userIDs []int64
	for i := range comments {
		userIDs = append(userIDs, comments[i].AuthorID)
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
	return strings.TrimSpace(u.Lastname + " " + u.Firstname)
}
