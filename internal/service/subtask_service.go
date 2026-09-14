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
	userRepo          repository.UserRepositoryInterface
	projectRepo       repository.ProjectRepositoryInterface
	projectMemberRepo repository.ProjectMemberRepositoryInterface
	realtimePublisher *KanbanRealtimePublisher
	notificationSvc   *KanbanNotificationService
	History           HistoryLogger
}

func NewSubtaskService(
	repo repository.SubtaskRepositoryInterface,
	permSvc *PermissionService,
	userRepo repository.UserRepositoryInterface,
	projectRepo repository.ProjectRepositoryInterface,
	projectMemberRepo repository.ProjectMemberRepositoryInterface,
	realtimePublisher *KanbanRealtimePublisher,
	notificationSvc *KanbanNotificationService,
) *SubtaskService {
	return &SubtaskService{
		repo:              repo,
		permSvc:           permSvc,
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
	subtasks, err := s.repo.GetSubtasks(ctx, cardID)
	if err != nil {
		return nil, err
	}
	if err := s.populateSubtaskUserNames(ctx, subtasks); err != nil {
		return nil, err
	}
	return subtasks, nil
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
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   projectID,
			Action:      "subtask.created",
			EntityType:  "subtask",
			EntityID:    st.ID,
			CardID:      cardID,
			EntityTitle: st.Title,
		})
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
		return nil, withNotFoundCode(err, apperr.CodeSubtaskNotFound)
	}
	if st.CardID != cardID {
		return nil, apperr.New(apperr.CodeSubtaskNotFound, "subtask not found")
	}

	prevTitle := st.Title
	oldPos := st.Position
	oldIsCompleted := st.Status == "done"
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
		if err := s.populateSubtaskUserName(ctx, updatedSt); err != nil {
			return nil, err
		}
	}
	if err == nil && updatedSt != nil {
		newIsCompleted := updatedSt.Status == "done"
		titleChanged := prevTitle != updatedSt.Title
		statusChanged := oldIsCompleted != newIsCompleted
		posChanged := oldPos != updatedSt.Position
		assigneeChanged := req.HasUserID && !sameOptionalID(oldUserID, updatedSt.UserID)

		action, before, after := subtaskUpdateAction(subtaskUpdateSummary{
			titleChanged: titleChanged, prevTitle: prevTitle, newTitle: updatedSt.Title,
			statusChanged:   statusChanged, nowCompleted: newIsCompleted,
			posChanged:      posChanged, oldPos: oldPos, newPos: updatedSt.Position,
			assigneeChanged: assigneeChanged, oldUserID: oldUserID, newUserID: updatedSt.UserID,
		})

		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   projectID,
			Action:      action,
			EntityType:  "subtask",
			EntityID:    subtaskID,
			CardID:      cardID,
			EntityTitle: updatedSt.Title,
			Before:      before,
			After:       after,
		})

		if statusChanged {
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
			// Notification for subtask assignment
			if s.notificationSvc != nil && updatedSt.UserID != nil {
				actorID := derefInt64(currentUserID(ctx))
				assigneeID := *updatedSt.UserID
				title := updatedSt.Title
				runDetached(ctx, notifyTimeout, "failed to notify kanban subtask assigned", func(ctx context.Context) error {
					s.notificationSvc.NotifySubtaskAssigned(ctx, projectID, cardID, actorID, assigneeID, title)

					if assigneeAddedToProject {
						proj, _ := s.projectRepo.GetProject(ctx, projectID)
						projName := ""
						if proj != nil {
							projName = proj.Name
						}
						s.notificationSvc.NotifyProjectUserAdded(ctx, projectID, actorID, assigneeID, projName)
					}
					return nil
				})
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
		return withNotFoundCode(err, apperr.CodeSubtaskNotFound)
	}
	if st.CardID != cardID {
		return apperr.New(apperr.CodeSubtaskNotFound, "subtask not found")
	}

	err = s.repo.DeleteSubtask(ctx, subtaskID)
	if err == nil {
		appendHistory(s.History, ctx, model.HistoryWrite{
			ProjectID:   projectID,
			Action:      "subtask.deleted",
			EntityType:  "subtask",
			EntityID:    subtaskID,
			CardID:      cardID,
			EntityTitle: st.Title,
		})
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

func (s *SubtaskService) populateSubtaskUserNames(ctx context.Context, subtasks []model.Subtask) error {
	userIDs := make([]int64, 0, len(subtasks))
	for i := range subtasks {
		if subtasks[i].UserID != nil {
			userIDs = append(userIDs, *subtasks[i].UserID)
		}
	}
	if len(userIDs) == 0 {
		return nil
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return err
	}
	userMap := make(map[int64]*model.User, len(users))
	for i := range users {
		userMap[users[i].ID] = &users[i]
	}
	for i := range subtasks {
		if subtasks[i].UserID == nil {
			continue
		}
		if u, ok := userMap[*subtasks[i].UserID]; ok {
			name := dto.UserDisplayName(*u)
			subtasks[i].UserName = &name
		}
	}

	return nil
}

func (s *SubtaskService) populateSubtaskUserName(ctx context.Context, subtask *model.Subtask) error {
	if subtask == nil || subtask.UserID == nil {
		return nil
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, []int64{*subtask.UserID})
	if err != nil {
		return err
	}
	if len(users) == 0 {
		return nil
	}

	name := dto.UserDisplayName(users[0])
	subtask.UserName = &name
	return nil
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
		return apperr.New(apperr.CodeUserNotFound, "user not found")
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

func sameOptionalID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
