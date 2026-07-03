package service

import (
	"context"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type LabelServiceInterface interface {
	GetLabels(ctx context.Context, boardID int64) ([]model.Label, error)
	CreateLabel(ctx context.Context, boardID int64, req dto.CreateLabelRequest) (*model.Label, error)
	DeleteLabel(ctx context.Context, labelID int64) error
	ToggleLabel(ctx context.Context, cardID int64, labelID int64) error
}

type LabelService struct {
	repo    repository.LabelRepositoryInterface
	permSvc *PermissionService
}

func NewLabelService(repo repository.LabelRepositoryInterface, permSvc *PermissionService) *LabelService {
	return &LabelService{
		repo:    repo,
		permSvc: permSvc,
	}
}

func (s *LabelService) GetLabels(ctx context.Context, boardID int64) ([]model.Label, error) {
	projectID, err := s.permSvc.GetProjectIDByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.GetLabels(ctx, boardID)
}

func (s *LabelService) CreateLabel(ctx context.Context, boardID int64, req dto.CreateLabelRequest) (*model.Label, error) {
	projectID, err := s.permSvc.GetProjectIDByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	l := &model.Label{
		Name:  req.Name,
		Color: req.Color,
	}
	return s.repo.CreateLabel(ctx, boardID, l)
}

func (s *LabelService) DeleteLabel(ctx context.Context, labelID int64) error {
	projectID, err := s.permSvc.GetProjectIDByLabel(ctx, labelID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}
	return s.repo.DeleteLabel(ctx, labelID)
}

func (s *LabelService) ToggleLabel(ctx context.Context, cardID int64, labelID int64) error {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}
	return s.repo.ToggleLabel(ctx, cardID, labelID)
}
