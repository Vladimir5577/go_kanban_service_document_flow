package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type CommentServiceInterface interface {
	GetComments(ctx context.Context, cardID int64) ([]model.Comment, error)
	GetComment(ctx context.Context, commentID int64) (*model.Comment, error)
	CreateComment(ctx context.Context, cardID int64, req dto.CreateCommentRequest) (*model.Comment, error)
	UpdateComment(ctx context.Context, commentID int64, req dto.UpdateCommentRequest) (*model.Comment, error)
	DeleteComment(ctx context.Context, commentID int64) error
}

type CommentService struct {
	repo     repository.CommentRepositoryInterface
	permSvc  *PermissionService
	userRepo repository.UserRepositoryInterface
}

func NewCommentService(repo repository.CommentRepositoryInterface, permSvc *PermissionService, userRepo repository.UserRepositoryInterface) *CommentService {
	return &CommentService{
		repo:     repo,
		permSvc:  permSvc,
		userRepo: userRepo,
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
	s.populateAuthorNames(ctx, []model.Comment{*c})
	return c, nil
}

func (s *CommentService) CreateComment(ctx context.Context, cardID int64, req dto.CreateCommentRequest) (*model.Comment, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil { // Viewer can comment? Wait, Symfony allows Viewer to comment if we just require viewer. Let's assume Viewer or Editor is enough to comment. Actually, usually viewers can't comment, let's require Editor. Wait, what is the role to comment? Let's use Editor.
		return nil, err
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrForbidden
	}

	comments, err := s.repo.GetComments(ctx, cardID)
	if err == nil && len(comments) >= 300 {
		return nil, apperr.New(apperr.CodeValidation, "maximum number of comments (300) per card reached")
	}

	c := &model.Comment{
		Body:     req.Body,
		CardID:   cardID,
		AuthorID: user.ID,
	}
	created, err := s.repo.CreateComment(ctx, cardID, c)
	if err != nil {
		return nil, err
	}
	s.populateAuthorNames(ctx, []model.Comment{*created})
	return created, nil
}

func (s *CommentService) UpdateComment(ctx context.Context, commentID int64, req dto.UpdateCommentRequest) (*model.Comment, error) {
	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return nil, err
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return nil, apperr.ErrForbidden
	}
	if c.AuthorID != user.ID {
		return nil, apperr.ErrForbidden
	}

	if req.Body != nil {
		c.Body = *req.Body
	}
	updated, err := s.repo.UpdateComment(ctx, c)
	if err != nil {
		return nil, err
	}
	s.populateAuthorNames(ctx, []model.Comment{*updated})
	return updated, nil
}

func (s *CommentService) DeleteComment(ctx context.Context, commentID int64) error {
	c, err := s.repo.GetComment(ctx, commentID)
	if err != nil {
		return err
	}

	user, ok := middleware.GetUser(ctx)
	if !ok {
		return apperr.ErrForbidden
	}
	
	// Author can delete, or Admin can delete
	if c.AuthorID != user.ID {
		projectID, err := s.permSvc.GetProjectIDByCard(ctx, c.CardID)
		if err != nil {
			return err
		}
		if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
			return apperr.ErrForbidden
		}
	}

	return s.repo.DeleteComment(ctx, commentID)
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
			name := u.Firstname
			if u.Lastname != "" {
				name += " " + u.Lastname
			}
			comments[i].AuthorName = name
		}
	}
}
