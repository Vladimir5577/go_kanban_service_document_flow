package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type SubtaskServiceInterface interface {
	GetSubtasks(ctx context.Context, cardID int64) ([]model.Subtask, error)
	CreateSubtask(ctx context.Context, cardID int64, req dto.CreateSubtaskRequest) (*model.Subtask, error)
	UpdateSubtask(ctx context.Context, subtaskID int64, req dto.UpdateSubtaskRequest) (*model.Subtask, error)
	DeleteSubtask(ctx context.Context, subtaskID int64) error
}

type SubtaskService struct {
	repo    repository.SubtaskRepositoryInterface
	permSvc *PermissionService
}

func NewSubtaskService(repo repository.SubtaskRepositoryInterface, permSvc *PermissionService) *SubtaskService {
	return &SubtaskService{
		repo:    repo,
		permSvc: permSvc,
	}
}

func (s *SubtaskService) GetSubtasks(ctx context.Context, cardID int64) ([]model.Subtask, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleViewer); err != nil {
		return nil, err
	}
	return s.repo.GetSubtasks(ctx, cardID)
}

func (s *SubtaskService) CreateSubtask(ctx context.Context, cardID int64, req dto.CreateSubtaskRequest) (*model.Subtask, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	subtasks, err := s.repo.GetSubtasks(ctx, cardID)
	if err == nil && len(subtasks) >= 100 {
		return nil, apperr.New(apperr.CodeValidation, "maximum number of subtasks (100) per card reached")
	}

	st := &model.Subtask{
		Title:  req.Title,
		CardID: cardID,
	}
	if req.Status != nil {
		st.Status = *req.Status
	}
	if req.Position != nil {
		st.Position = *req.Position
	}
	return s.repo.CreateSubtask(ctx, cardID, st)
}

func (s *SubtaskService) UpdateSubtask(ctx context.Context, subtaskID int64, req dto.UpdateSubtaskRequest) (*model.Subtask, error) {
	projectID, err := s.permSvc.GetProjectIDBySubtask(ctx, subtaskID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	st := &model.Subtask{ID: subtaskID}
	if req.Title != nil {
		st.Title = *req.Title
	}
	if req.Status != nil {
		st.Status = *req.Status
	}
	if req.Position != nil {
		st.Position = *req.Position
	}
	return s.repo.UpdateSubtask(ctx, subtaskID, st)
}

func (s *SubtaskService) DeleteSubtask(ctx context.Context, subtaskID int64) error {
	projectID, err := s.permSvc.GetProjectIDBySubtask(ctx, subtaskID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}

	return s.repo.DeleteSubtask(ctx, subtaskID)
}
