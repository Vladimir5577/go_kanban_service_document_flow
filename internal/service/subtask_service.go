package service

import (
	"context"
	"errors"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type SubtaskServiceInterface interface {
	GetSubtasks(ctx context.Context, cardID int64) ([]model.Subtask, error)
	CreateSubtask(ctx context.Context, cardID int64, req dto.CreateSubtaskRequest) (*model.Subtask, error)
	UpdateSubtask(ctx context.Context, cardID int64, subtaskID int64, req dto.UpdateSubtaskRequest) (*model.Subtask, error)
	DeleteSubtask(ctx context.Context, cardID int64, subtaskID int64) error
}

type SubtaskService struct {
	repo              repository.SubtaskRepositoryInterface
	permSvc           *PermissionService
	activityRepo      repository.ActivityRepositoryInterface
	userRepo          repository.UserRepositoryInterface
	projectRepo       repository.ProjectRepositoryInterface
	projectMemberRepo repository.ProjectMemberRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
}

func NewSubtaskService(
	repo repository.SubtaskRepositoryInterface,
	permSvc *PermissionService,
	activityRepo repository.ActivityRepositoryInterface,
	userRepo repository.UserRepositoryInterface,
	projectRepo repository.ProjectRepositoryInterface,
	projectMemberRepo repository.ProjectMemberRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
	notificationSvc *KanbanNotificationService,
) *SubtaskService {
	return &SubtaskService{
		repo:              repo,
		permSvc:           permSvc,
		activityRepo:      activityRepo,
		userRepo:          userRepo,
		projectRepo:       projectRepo,
		projectMemberRepo: projectMemberRepo,
		realtimePublisher: realtimePublisher,
		notificationSvc:   notificationSvc,
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
	st, err = s.repo.CreateSubtask(ctx, cardID, st)
	if err == nil {
		s.logActivity(ctx, cardID, "subtask_added", nil, &req.Title)
		if s.realtimePublisher != nil {
			s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
				patch, err := s.realtimePublisher.BuildChecklistCounters(ctx, cardID)
				if err != nil {
					return err
				}
				return s.realtimePublisher.PublishCardPatchByID(ctx, cardID, patch, realtimeSenderID(ctx))
			})
		}
	}
	return st, err
}

func (s *SubtaskService) UpdateSubtask(ctx context.Context, cardID int64, subtaskID int64, req dto.UpdateSubtaskRequest) (*model.Subtask, error) {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return nil, err
	}

	st, err := s.repo.GetSubtask(ctx, subtaskID)
	if err != nil {
		return nil, err
	}
	if st.CardID != cardID {
		return nil, apperr.ErrNotFound
	}

	var oldIsCompleted bool
	if st.Status == "done" {
		oldIsCompleted = true
	}
	oldUserID := st.UserID

	if req.Title != nil {
		st.Title = *req.Title
	}
	if req.Status != nil {
		st.Status = *req.Status
	}
	if req.IsCompleted != nil {
		if *req.IsCompleted {
			st.Status = "done"
		} else {
			st.Status = "todo"
		}
	}
	if req.Position != nil {
		st.Position = *req.Position
	}
	var assigneeAddedToProject bool
	if req.HasUserID && req.UserID != nil {
		if _, err := s.projectMemberRepo.GetProjectMember(ctx, projectID, *req.UserID); err != nil && errors.Is(err, apperr.ErrNotFound) {
			assigneeAddedToProject = true
		}
		if err := s.ensureSubtaskAssignee(ctx, projectID, req.UserID); err != nil {
			return nil, err
		}
		st.UserID = req.UserID
	}
	updatedSt, err := s.repo.UpdateSubtask(ctx, subtaskID, st)
	if err == nil && updatedSt != nil {
		var newIsCompleted bool
		if updatedSt.Status == "done" {
			newIsCompleted = true
		}

		if oldIsCompleted != newIsCompleted {
			if newIsCompleted {
				s.logActivity(ctx, updatedSt.CardID, "subtask_completed", nil, &updatedSt.Title)
			} else {
				s.logActivity(ctx, updatedSt.CardID, "subtask_reopened", nil, &updatedSt.Title)
			}
			if s.realtimePublisher != nil {
				s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
					patch, err := s.realtimePublisher.BuildChecklistCounters(ctx, updatedSt.CardID)
					if err != nil {
						return err
					}
					return s.realtimePublisher.PublishCardPatchByID(ctx, updatedSt.CardID, patch, realtimeSenderID(ctx))
				})
			}
		}

		if req.HasUserID && !sameOptionalID(oldUserID, updatedSt.UserID) {
			if oldUserID != nil {
				oldValue := s.subtaskAssigneeActivityValue(ctx, *oldUserID, updatedSt.Title)
				s.logActivity(ctx, updatedSt.CardID, "subtask_unassigned", &oldValue, nil)
			}
			if updatedSt.UserID != nil {
				newValue := s.subtaskAssigneeActivityValue(ctx, *updatedSt.UserID, updatedSt.Title)
				s.logActivity(ctx, updatedSt.CardID, "subtask_assigned", nil, &newValue)
			}

			// Notification for subtask assignment
			if s.notificationSvc != nil && updatedSt.UserID != nil {
				actorID := currentUserID(ctx)
				s.notificationSvc.NotifySubtaskAssigned(ctx, projectID, cardID, derefInt64(actorID), *updatedSt.UserID, updatedSt.Title)

				if assigneeAddedToProject {
					proj, _ := s.projectRepo.GetProject(ctx, projectID)
					projName := ""
					if proj != nil {
						projName = proj.Name
					}
					s.notificationSvc.NotifyProjectUserAdded(ctx, projectID, derefInt64(actorID), *updatedSt.UserID, projName)
				}
			}
		}
	}
	return updatedSt, err
}

func (s *SubtaskService) DeleteSubtask(ctx context.Context, cardID int64, subtaskID int64) error {
	projectID, err := s.permSvc.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return err
	}
	if err := s.permSvc.RequireRole(ctx, projectID, RoleEditor); err != nil {
		return err
	}

	st, err := s.repo.GetSubtask(ctx, subtaskID)
	if err != nil {
		return err
	}
	if st.CardID != cardID {
		return apperr.ErrNotFound
	}

	err = s.repo.DeleteSubtask(ctx, subtaskID)
	if err == nil {
		s.logActivity(ctx, st.CardID, "subtask_removed", &st.Title, nil)
		if s.realtimePublisher != nil {
			s.realtimePublisher.TryPublish(ctx, func(ctx context.Context) error {
				patch, err := s.realtimePublisher.BuildChecklistCounters(ctx, st.CardID)
				if err != nil {
					return err
				}
				return s.realtimePublisher.PublishCardPatchByID(ctx, st.CardID, patch, realtimeSenderID(ctx))
			})
		}
	}
	return err
}

func (s *SubtaskService) ensureSubtaskAssignee(ctx context.Context, projectID int64, userID *int64) error {
	if userID == nil {
		return nil
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, []int64{*userID})
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return apperr.ErrNotFound
	}

	project, err := s.projectRepo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if project.OwnerID == *userID {
		return nil
	}

	if _, err := s.projectMemberRepo.GetProjectMember(ctx, projectID, *userID); err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return s.projectMemberRepo.AddMember(ctx, projectID, model.ProjectUser{
				KanbanProjectID: projectID,
				UserID:          *userID,
				Role:            string(RoleViewer),
			})
		}
		return err
	}

	return nil
}

func (s *SubtaskService) logActivity(ctx context.Context, cardID int64, action string, oldValue, newValue *string) {
	_ = s.activityRepo.LogActivity(ctx, cardID, currentUserID(ctx), action, oldValue, newValue)
}

func (s *SubtaskService) subtaskAssigneeActivityValue(ctx context.Context, userID int64, subtaskTitle string) string {
	name := ""
	if users, err := s.userRepo.GetUsersByIDs(ctx, []int64{userID}); err == nil && len(users) > 0 {
		name = users[0].Firstname
		if users[0].Lastname != "" {
			if name != "" {
				name += " "
			}
			name += users[0].Lastname
		}
	}
	if name == "" {
		name = "Пользователь"
	}
	return name + " (подзадача: " + subtaskTitle + ")"
}

func sameOptionalID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
