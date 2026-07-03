package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type ColumnServiceInterface interface {
	CreateColumn(ctx context.Context, boardID int64, req dto.CreateColumnRequest) (*model.Column, error)
	// GetColumn is not in the mock repo yet, but we'll simulate update
	UpdateColumn(ctx context.Context, columnID int64, req dto.UpdateColumnRequest) (*model.Column, error)
	DeleteColumn(ctx context.Context, columnID int64) error
}

type ColumnService struct {
	repo    repository.ColumnRepositoryInterface
	permSvc *PermissionService
}

func NewColumnService(repo repository.ColumnRepositoryInterface, permSvc *PermissionService) *ColumnService {
	return &ColumnService{
		repo:    repo,
		permSvc: permSvc,
	}
}

func (s *ColumnService) CreateColumn(ctx context.Context, boardID int64, req dto.CreateColumnRequest) (*model.Column, error) {
	projectID, err := s.permSvc.GetProjectIDByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	c := &model.Column{
		Title: req.Title,
	}
	if req.HeaderColor != nil {
		c.HeaderColor = *req.HeaderColor
	}
	if req.Position != nil {
		c.Position = *req.Position
	}
	return s.repo.CreateColumn(ctx, boardID, c)
}

func (s *ColumnService) UpdateColumn(ctx context.Context, columnID int64, req dto.UpdateColumnRequest) (*model.Column, error) {
	projectID, err := s.permSvc.GetProjectIDByColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	c, err := s.repo.GetColumn(ctx, columnID)
	if err != nil {
		return nil, err
	}
	
	if req.Title != nil {
		c.Title = *req.Title
	}
	if req.HeaderColor != nil {
		c.HeaderColor = *req.HeaderColor
	}
	if req.Position != nil {
		c.Position = *req.Position
	}
	return s.repo.UpdateColumn(ctx, c)
}

func (s *ColumnService) DeleteColumn(ctx context.Context, columnID int64) error {
	projectID, err := s.permSvc.GetProjectIDByColumn(ctx, columnID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}

	hasCards, err := s.repo.HasCardsByColumn(ctx, columnID)
	if err == nil && hasCards {
		return apperr.New(apperr.CodeValidation, "cannot delete column with active cards")
	}

	return s.repo.DeleteColumn(ctx, columnID)
}
