package service

import (
	"context"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type ProjectFolderServiceInterface interface {
	GetProjectFolders(ctx context.Context) ([]model.ProjectUserFolder, error)
}

type ProjectFolderService struct {
	repo    repository.ProjectFolderRepositoryInterface
	permSvc *PermissionService
}

func NewProjectFolderService(repo repository.ProjectFolderRepositoryInterface, permSvc *PermissionService) *ProjectFolderService {
	return &ProjectFolderService{
		repo:    repo,
		permSvc: permSvc,
	}
}

func (s *ProjectFolderService) GetProjectFolders(ctx context.Context) ([]model.ProjectUserFolder, error) {
	return s.repo.GetProjectFolders(ctx)
}
