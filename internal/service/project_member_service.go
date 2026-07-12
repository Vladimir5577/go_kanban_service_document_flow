package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

type ProjectMemberServiceInterface interface {
	ReplaceMembers(ctx context.Context, projectID int64, reqs []dto.AddProjectMemberRequest) error
	UpdateMemberRole(ctx context.Context, projectID int64, userID int64, req dto.UpdateProjectMemberRequest) error
	RemoveMember(ctx context.Context, projectID int64, userID int64) error
}

type ProjectMemberService struct {
	repo                 repository.ProjectMemberRepositoryInterface
	userRepo             repository.UserRepositoryInterface
	permSvc              *PermissionService
	notificationSvc      *KanbanNotificationService
}

func NewProjectMemberService(repo repository.ProjectMemberRepositoryInterface, userRepo repository.UserRepositoryInterface, permSvc *PermissionService, notificationSvc *KanbanNotificationService) *ProjectMemberService {
	return &ProjectMemberService{
		repo:            repo,
		userRepo:        userRepo,
		permSvc:         permSvc,
		notificationSvc: notificationSvc,
	}
}

func (s *ProjectMemberService) ReplaceMembers(ctx context.Context, projectID int64, reqs []dto.AddProjectMemberRequest) error {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}

	project, err := s.permSvc.projectRepo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}

	// For notifications: remember who was already a member
	existingMembers, _ := s.repo.GetMembers(ctx, projectID)
	existingUserIDs := map[int64]bool{}
	for _, m := range existingMembers {
		existingUserIDs[m.UserID] = true
	}

	membersByUserID := make(map[int64]model.ProjectUser, len(reqs)+1)
	for _, req := range reqs {
		if req.UserID <= 0 {
			continue
		}

		role := RoleAdmin
		if req.UserID != project.OwnerID {
			parsedRole, err := parseProjectMemberRole(req.Role)
			if err != nil {
				return apperr.New(apperr.CodeValidation, fmt.Sprintf("invalid role for user %d", req.UserID))
			}
			role = parsedRole
		}

		membersByUserID[req.UserID] = model.ProjectUser{
			KanbanProjectID: projectID,
			UserID:          req.UserID,
			Role:            string(role),
			FolderID:        req.FolderID,
		}
	}

	ownerMember := membersByUserID[project.OwnerID]
	ownerMember.KanbanProjectID = projectID
	ownerMember.UserID = project.OwnerID
	ownerMember.Role = string(RoleAdmin)
	membersByUserID[project.OwnerID] = ownerMember

	members := sortedProjectMembers(membersByUserID)
	if len(members) == 0 {
		return apperr.New(apperr.CodeValidation, "members list empty")
	}
	if err := s.requireExistingUsers(ctx, members); err != nil {
		return err
	}

	if err := s.repo.ReplaceMembers(ctx, projectID, members); err != nil {
		return err
	}

	// Notify newly added members
	if s.notificationSvc != nil {
		actor, _ := middleware.GetUser(ctx)
		for _, m := range members {
			if !existingUserIDs[m.UserID] && m.UserID != actor.ID {
				s.notificationSvc.NotifyProjectUserAdded(ctx, projectID, actor.ID, m.UserID, project.Name)
			}
		}
	}

	return nil
}

func (s *ProjectMemberService) UpdateMemberRole(ctx context.Context, projectID int64, userID int64, req dto.UpdateProjectMemberRequest) error {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}

	project, err := s.permSvc.projectRepo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if userID == project.OwnerID {
		return apperr.New(apperr.CodeValidation, "owner role immutable")
	}

	roleValue := ""
	if req.Role != nil {
		roleValue = *req.Role
	}
	role, err := parseProjectMemberRole(roleValue)
	if err != nil {
		return err
	}

	if err := s.requireProjectMember(ctx, projectID, userID); err != nil {
		return err
	}
	return s.repo.UpdateMemberRole(ctx, projectID, userID, string(role))
}

func (s *ProjectMemberService) RemoveMember(ctx context.Context, projectID int64, userID int64) error {
	if err := s.permSvc.RequireRole(ctx, projectID, RoleAdmin); err != nil {
		return err
	}

	currentUser, ok := middleware.GetUser(ctx)
	if !ok {
		return apperr.ErrUnauthorized
	}
	if userID == currentUser.ID {
		return apperr.New(apperr.CodeValidation, "cannot remove self")
	}

	project, err := s.permSvc.projectRepo.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	if userID == project.OwnerID {
		return apperr.New(apperr.CodeValidation, "cannot remove owner")
	}

	if err := s.requireProjectMember(ctx, projectID, userID); err != nil {
		return err
	}
	if err := s.repo.RemoveMember(ctx, projectID, userID); err != nil {
		return err
	}

	// Notify the removed user
	if s.notificationSvc != nil {
		actor, _ := middleware.GetUser(ctx)
		s.notificationSvc.NotifyProjectUserRemoved(ctx, projectID, actor.ID, userID, project.Name)
	}

	return nil
}

func parseProjectMemberRole(value string) (Role, error) {
	role := Role(strings.TrimSpace(value))
	switch role {
	case RoleViewer, RoleEditor, RoleAdmin:
		return role, nil
	default:
		return "", apperr.New(apperr.CodeValidation, "invalid role")
	}
}

func sortedProjectMembers(membersByUserID map[int64]model.ProjectUser) []model.ProjectUser {
	userIDs := make([]int64, 0, len(membersByUserID))
	for userID := range membersByUserID {
		userIDs = append(userIDs, userID)
	}
	sort.Slice(userIDs, func(i, j int) bool {
		return userIDs[i] < userIDs[j]
	})

	members := make([]model.ProjectUser, 0, len(userIDs))
	for _, userID := range userIDs {
		members = append(members, membersByUserID[userID])
	}
	return members
}

func (s *ProjectMemberService) requireExistingUsers(ctx context.Context, members []model.ProjectUser) error {
	userIDs := make([]int64, 0, len(members))
	for _, member := range members {
		userIDs = append(userIDs, member.UserID)
	}

	users, err := s.userRepo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return err
	}
	existing := make(map[int64]struct{}, len(users))
	for _, user := range users {
		existing[user.ID] = struct{}{}
	}

	for _, userID := range userIDs {
		if _, ok := existing[userID]; !ok {
			return apperr.New(apperr.CodeValidation, fmt.Sprintf("user not found: %d", userID))
		}
	}
	return nil
}

func (s *ProjectMemberService) requireProjectMember(ctx context.Context, projectID int64, userID int64) error {
	if _, err := s.repo.GetProjectMember(ctx, projectID, userID); err != nil {
		if errors.Is(err, apperr.ErrNotFound) {
			return apperr.ErrNotFound
		}
		return err
	}
	return nil
}
