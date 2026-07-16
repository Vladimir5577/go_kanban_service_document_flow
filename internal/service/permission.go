package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/repository"
	"go_kanban_service/internal/repository/dbgen"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Role string

const (
	RoleViewer Role = "KANBAN_VIEWER"
	RoleEditor Role = "KANBAN_EDITOR"
	RoleAdmin  Role = "KANBAN_ADMIN"
)

var roleLevels = map[Role]int{
	RoleViewer: 1,
	RoleEditor: 2,
	RoleAdmin:  3,
}

type PermissionService struct {
	db          *pgxpool.Pool
	projectRepo repository.ProjectRepositoryInterface
	memberRepo  repository.ProjectMemberRepositoryInterface
}

func NewPermissionService(db *pgxpool.Pool, projectRepo repository.ProjectRepositoryInterface, memberRepo repository.ProjectMemberRepositoryInterface) *PermissionService {
	return &PermissionService{
		db:          db,
		projectRepo: projectRepo,
		memberRepo:  memberRepo,
	}
}

// GetMemberRole возвращает роль пользователя в проекте, или ошибку, если у него нет доступа
func (s *PermissionService) GetMemberRole(ctx context.Context, projectID int64) (Role, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return "", apperr.ErrUnauthorized
	}

	// 1. Владелец проекта всегда ADMIN
	project, err := s.projectRepo.GetProject(ctx, projectID)
	if err != nil {
		return "", withNotFoundCode(err, apperr.CodeProjectNotFound)
	}
	if project.OwnerID == user.ID {
		return RoleAdmin, nil
	}

	// 2. Ищем пользователя в участниках проекта (канбан роли)
	member, err := s.memberRepo.GetProjectMember(ctx, projectID, user.ID)
	if err != nil {
		// Если не найден в БД - значит доступа нет
		return "", accessDenied()
	}

	return Role(member.Role), nil
}

// RequireRole проверяет, есть ли у пользователя требуемый уровень прав
func (s *PermissionService) RequireRole(ctx context.Context, projectID int64, minRole Role) error {
	userRole, err := s.GetMemberRole(ctx, projectID)
	if err != nil {
		return err
	}

	requiredLevel, ok := roleLevels[minRole]
	if !ok {
		return accessDenied()
	}

	userLevel, ok := roleLevels[userRole]
	if !ok {
		return accessDenied()
	}

	if userLevel < requiredLevel {
		return accessDenied()
	}

	return nil
}

func (s *PermissionService) GetProjectIDByBoard(ctx context.Context, boardID int64) (int64, error) {
	queries := dbgen.New(s.db)
	b, err := queries.GetBoard(ctx, boardID)
	if err != nil {
		return 0, withNotFoundCode(repository.NormalizeError(err), apperr.CodeBoardNotFound)
	}
	return b.KanbanProjectID, nil
}

func (s *PermissionService) GetProjectIDByColumn(ctx context.Context, columnID int64) (int64, error) {
	queries := dbgen.New(s.db)
	projectID, err := queries.GetProjectIDByColumn(ctx, columnID)
	if err != nil {
		return 0, withNotFoundCode(repository.NormalizeError(err), apperr.CodeColumnNotFound)
	}
	return projectID, nil
}

func (s *PermissionService) GetProjectIDByCard(ctx context.Context, cardID int64) (int64, error) {
	queries := dbgen.New(s.db)
	projectID, err := queries.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return 0, withNotFoundCode(repository.NormalizeError(err), apperr.CodeCardNotFound)
	}
	return projectID, nil
}

func (s *PermissionService) GetProjectIDBySubtask(ctx context.Context, subtaskID int64) (int64, error) {
	queries := dbgen.New(s.db)
	projectID, err := queries.GetProjectIDBySubtask(ctx, subtaskID)
	if err != nil {
		return 0, withNotFoundCode(repository.NormalizeError(err), apperr.CodeSubtaskNotFound)
	}
	return projectID, nil
}

func (s *PermissionService) GetProjectIDByLabel(ctx context.Context, labelID int64) (int64, error) {
	queries := dbgen.New(s.db)
	projectID, err := queries.GetProjectIDByLabel(ctx, labelID)
	if err != nil {
		return 0, withNotFoundCode(repository.NormalizeError(err), apperr.CodeLabelNotFound)
	}
	return projectID, nil
}
