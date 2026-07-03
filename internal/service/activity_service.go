package service

import (
	"context"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type ActivityServiceInterface interface {
	GetActivities(ctx context.Context, cardID int64) ([]model.Activity, error)
}

type ActivityService struct {
	repo    repository.ActivityRepositoryInterface
	permSvc *PermissionService
}

func NewActivityService(repo repository.ActivityRepositoryInterface, permSvc *PermissionService) *ActivityService {
	return &ActivityService{
		repo:    repo,
		permSvc: permSvc,
	}
}

func (s *ActivityService) GetActivities(ctx context.Context, cardID int64) ([]model.Activity, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.GetActivities(ctx, cardID)
}
