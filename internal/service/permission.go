package service

import (
	"context"
	"time"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
	"go_kanban_service/internal/repository/dbgen"

	"github.com/jackc/pgx/v5/pgtype"
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

// CardAccess — разрешённый контекст карточки: проект, доска с названием,
// заголовок колонки, владелец и роль текущего пользователя. Всё это добывается
// одним запросом вместо цепочки GetProjectIDByCard → GetProject → GetColumn.
type CardAccess struct {
	ProjectID   int64
	BoardID     int64
	BoardTitle  string
	ColumnTitle string
	OwnerID     int64
	Role        Role
	ParentID    *int64
	IsArchived  bool
	CardTitle   string
	// ColumnID и Position — у подзадачи колонка родителя и собственная позиция.
	ColumnID     int64
	Position     float64
	CompletedAt  *time.Time
	DoneColumnID *int64
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

	row, err := dbgen.New(s.db).GetCardContext(ctx, dbgen.GetCardContextParams{ID: cardID, UserID: user.ID})
	if err != nil {
		return CardAccess{}, withNotFoundCode(repository.NormalizeError(err), apperr.CodeCardNotFound)
	}
	role, err := checkContextRole(user.ID, row.OwnerID, row.MemberRole, row.ProjectDeletedAt, minRole)
	if err != nil {
		return CardAccess{}, err
	}

	acc := CardAccess{
		ProjectID:   row.KanbanProjectID,
		BoardID:     row.BoardID,
		BoardTitle:  row.BoardTitle,
		ColumnTitle: row.ColumnTitle,
		OwnerID:     row.OwnerID,
		Role:        role,
		IsArchived:  row.IsArchived,
		CardTitle:   row.CardTitle,
		ColumnID:    row.ColumnID,
		Position:    row.Position,
	}
	if row.ParentID.Valid {
		id := row.ParentID.Int64
		acc.ParentID = &id
	}
	if row.CompletedAt.Valid {
		t := row.CompletedAt.Time
		acc.CompletedAt = &t
	}
	if row.DoneColumnID.Valid {
		id := row.DoneColumnID.Int64
		acc.DoneColumnID = &id
	}
	return acc, nil
}

// checkContextRole — общая часть проверок по контексту карточки и колонки:
// проект в мягком удалении, роль по владельцу и членству, требуемый уровень.
func checkContextRole(userID, ownerID int64, memberRole pgtype.Text, projectDeletedAt pgtype.Timestamptz, minRole Role) (Role, error) {
	// Проект в мягком удалении: код тот же, что отдавал GetProject.
	if projectDeletedAt.Valid {
		return "", apperr.New(apperr.CodeProjectNotFound, string(apperr.CodeProjectNotFound))
	}
	// Членство приехало тем же запросом. NULL означает ровно то же, что раньше
	// означал промах GetProjectMember, — членства нет, и решает это resolveRole.
	var member *model.ProjectUser
	if memberRole.Valid {
		member = &model.ProjectUser{Role: memberRole.String}
	}
	role, err := resolveRole(userID, ownerID, member)
	if err != nil {
		return "", err
	}
	if !hasRole(role, minRole) {
		return "", accessDenied()
	}
	return role, nil
}

// ColumnAccess — разрешённый контекст колонки: всё, что нужно созданию карточки.
type ColumnAccess struct {
	ProjectID       int64
	BoardID         int64
	BoardTitle      string
	ColumnTitle     string
	PrependPosition float64
}

// RequireColumnRole — RequireCardRole от колонки: контекст одним запросом и
// сразу проверка прав.
func (s *PermissionService) RequireColumnRole(ctx context.Context, columnID int64, minRole Role) (ColumnAccess, error) {
	user, ok := middleware.GetUser(ctx)
	if !ok {
		return ColumnAccess{}, apperr.ErrUnauthorized
	}
	row, err := dbgen.New(s.db).GetColumnContext(ctx, dbgen.GetColumnContextParams{ID: columnID, UserID: user.ID})
	if err != nil {
		return ColumnAccess{}, withNotFoundCode(repository.NormalizeError(err), apperr.CodeColumnNotFound)
	}
	if _, err := checkContextRole(user.ID, row.OwnerID, row.MemberRole, row.ProjectDeletedAt, minRole); err != nil {
		return ColumnAccess{}, err
	}
	return ColumnAccess{
		ProjectID:       row.KanbanProjectID,
		BoardID:         row.BoardID,
		BoardTitle:      row.BoardTitle,
		ColumnTitle:     row.ColumnTitle,
		PrependPosition: row.PrependPosition,
	}, nil
}

// RequireRootCardRole — то же, что RequireCardRole, но для операций, которые
// у подзадачи смысла не имеют: комментарии, вложения, метки. Родительство
// приезжает тем же запросом, отдельного чтения карточки не нужно.
func (s *PermissionService) RequireRootCardRole(ctx context.Context, cardID int64, minRole Role) (CardAccess, error) {
	acc, err := s.RequireCardRole(ctx, cardID, minRole)
	if err != nil {
		return CardAccess{}, err
	}
	if acc.ParentID != nil {
		return CardAccess{}, apperr.New(apperr.CodeValidation, "operation not allowed on child card")
	}
	return acc, nil
}

func (s *PermissionService) GetProjectIDByBoard(ctx context.Context, boardID int64) (int64, error) {
	queries := dbgen.New(s.db)
	b, err := queries.GetBoard(ctx, boardID)
	if err != nil {
		return 0, withNotFoundCode(repository.NormalizeError(err), apperr.CodeBoardNotFound)
	}
	return b.KanbanProjectID, nil
}

func (s *PermissionService) GetProjectIDByCard(ctx context.Context, cardID int64) (int64, error) {
	queries := dbgen.New(s.db)
	projectID, err := queries.GetProjectIDByCard(ctx, cardID)
	if err != nil {
		return 0, withNotFoundCode(repository.NormalizeError(err), apperr.CodeCardNotFound)
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
