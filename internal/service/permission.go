package service

import (
	"context"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
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

// resolveRole решает роль по владельцу и членству: владелец проекта всегда
// ADMIN, иначе роль берётся из членства, а его отсутствие означает отказ.
// Чистая функция — вся логика доступа собрана здесь и проверяется тестом.
func resolveRole(userID, ownerID int64, member *model.ProjectUser) (Role, error) {
	if userID == ownerID {
		return RoleAdmin, nil
	}
	if member == nil {
		return "", accessDenied()
	}
	return Role(member.Role), nil
}

// hasRole сообщает, дотягивает ли роль до требуемой. Неизвестная роль с любой
// стороны — отказ, а не «пропустим на всякий случай».
func hasRole(userRole, minRole Role) bool {
	requiredLevel, ok := roleLevels[minRole]
	if !ok {
		return false
	}
	userLevel, ok := roleLevels[userRole]
	if !ok {
		return false
	}
	return userLevel >= requiredLevel
}

// GetMemberRole возвращает роль пользователя в проекте, или ошибку, если у него нет доступа
func (s *PermissionService) GetMemberRole(ctx context.Context, projectID int64) (Role, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return "", apperr.ErrUnauthorized
	}

	project, err := s.projectRepo.GetProject(ctx, projectID)
	if err != nil {
		return "", withNotFoundCode(err, apperr.CodeProjectNotFound)
	}

	var member *model.ProjectUser
	if project.OwnerID != user.ID {
		// Ошибку не разбираем: и «нет строки», и сбой запроса означают,
		// что членство подтвердить нечем — resolveRole ответит отказом.
		member, _ = s.memberRepo.GetProjectMember(ctx, projectID, user.ID)
	}

	return resolveRole(user.ID, project.OwnerID, member)
}

// RequireRole проверяет, есть ли у пользователя требуемый уровень прав
func (s *PermissionService) RequireRole(ctx context.Context, projectID int64, minRole Role) error {
	userRole, err := s.GetMemberRole(ctx, projectID)
	if err != nil {
		return err
	}
	if !hasRole(userRole, minRole) {
		return accessDenied()
	}
	return nil
}

// CardAccess — разрешённый контекст карточки: проект, доска, заголовок колонки,
// владелец и роль текущего пользователя. Всё это добывается одним запросом
// вместо цепочки GetProjectIDByCard → GetProject → GetColumn.
type CardAccess struct {
	ProjectID   int64
	BoardID     int64
	ColumnTitle string
	OwnerID     int64
	Role        Role
}

// IsOwner — текущий пользователь владелец проекта.
func (a CardAccess) IsOwner(userID int64) bool { return userID == a.OwnerID }

// RequireCardRole резолвит контекст карточки и сразу проверяет права.
// Намеренно одна функция: контекст нельзя получить в обход проверки, поэтому
// её невозможно забыть в новом вызывающем коде.
func (s *PermissionService) RequireCardRole(ctx context.Context, cardID int64, minRole Role) (CardAccess, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return CardAccess{}, apperr.ErrUnauthorized
	}

	row, err := dbgen.New(s.db).GetCardContext(ctx, cardID)
	if err != nil {
		return CardAccess{}, withNotFoundCode(repository.NormalizeError(err), apperr.CodeCardNotFound)
	}
	// Проект в мягком удалении: код тот же, что отдавал GetProject.
	if row.ProjectDeletedAt.Valid {
		return CardAccess{}, apperr.New(apperr.CodeProjectNotFound, string(apperr.CodeProjectNotFound))
	}

	var member *model.ProjectUser
	if row.OwnerID != user.ID {
		member, _ = s.memberRepo.GetProjectMember(ctx, row.KanbanProjectID, user.ID)
	}

	role, err := resolveRole(user.ID, row.OwnerID, member)
	if err != nil {
		return CardAccess{}, err
	}
	if !hasRole(role, minRole) {
		return CardAccess{}, accessDenied()
	}

	return CardAccess{
		ProjectID:   row.KanbanProjectID,
		BoardID:     row.BoardID,
		ColumnTitle: row.ColumnTitle,
		OwnerID:     row.OwnerID,
		Role:        role,
	}, nil
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
