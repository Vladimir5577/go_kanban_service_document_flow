package service

import (
	"context"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type ProjectMemberServiceInterface interface {
	ReplaceMembers(ctx context.Context, projectID int64, reqs []dto.AddProjectMemberRequest) error
	UpdateMemberRole(ctx context.Context, projectID int64, userID int64, req dto.UpdateProjectMemberRequest) error
	RemoveMember(ctx context.Context, projectID int64, userID int64) error
}

type ProjectMemberService struct {
	repo    repository.ProjectMemberRepositoryInterface
	permSvc *PermissionService
}

func NewProjectMemberService(repo repository.ProjectMemberRepositoryInterface, permSvc *PermissionService) *ProjectMemberService {
	return &ProjectMemberService{
		repo:    repo,
		permSvc: permSvc,
	}
}

func (s *ProjectMemberService) ReplaceMembers(ctx context.Context, projectID int64, reqs []dto.AddProjectMemberRequest) error {
	var members []model.ProjectUser
	for _, req := range reqs {
		members = append(members, model.ProjectUser{
			UserID:   req.UserID,
			Role:     req.Role,
			FolderID: req.FolderID,
		})
	}
	return s.repo.ReplaceMembers(ctx, projectID, members)
}

func (s *ProjectMemberService) UpdateMemberRole(ctx context.Context, projectID int64, userID int64, req dto.UpdateProjectMemberRequest) error {
	role := ""
	if req.Role != nil {
		role = *req.Role
	}
	return s.repo.UpdateMemberRole(ctx, projectID, userID, role)
}

func (s *ProjectMemberService) RemoveMember(ctx context.Context, projectID int64, userID int64) error {
	return s.repo.RemoveMember(ctx, projectID, userID)
}
