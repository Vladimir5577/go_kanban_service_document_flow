package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
)

type ProjectRepositoryInterface interface {
	ListProjects(ctx context.Context, f model.ProjectListFilters) (*model.ProjectListPage, error)
	CreateProject(ctx context.Context, p *model.Project) (*model.Project, error)
	GetProject(ctx context.Context, id int64) (*model.Project, error)
	UpdateProject(ctx context.Context, p *model.Project) (*model.Project, error)
	DeleteProject(ctx context.Context, id int64) error
	GetNavProjectsForUser(ctx context.Context, userID int64) ([]model.NavProject, error)
}

type ProjectRepository struct {
	Db *pgxpool.Pool
}

func NewProjectRepository(db *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{
		Db: db,
	}
}

func (r *ProjectRepository) ListProjects(ctx context.Context, f model.ProjectListFilters) (*model.ProjectListPage, error) {
	page := f.Page
	if page < 1 {
		page = 1
	}
	limit := f.Limit
	if limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	status := f.Status
	if status == "" {
		status = model.ProjectStatusActive
	}

	where := "TRUE"
	args := make([]any, 0, 4)
	argN := 1

	switch status {
	case model.ProjectStatusActive:
		where += " AND p.deleted_at IS NULL"
	case model.ProjectStatusDeleted:
		where += " AND p.deleted_at IS NOT NULL"
	case model.ProjectStatusAll:
		// no status filter
	default:
		where += " AND p.deleted_at IS NULL"
	}

	if search := strings.TrimSpace(f.Search); search != "" {
		where += fmt.Sprintf(" AND (p.name ILIKE $%d OR COALESCE(p.description, '') ILIKE $%d)", argN, argN)
		args = append(args, "%"+search+"%")
		argN++
	}

	countQuery := `SELECT COUNT(*) FROM kanban_project p WHERE ` + where
	var total int64
	if err := r.Db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, NormalizeError(err)
	}

	orderBySQL, ok := map[string]string{
		"name":          "p.name",
		"created_at":    "p.created_at",
		"members_count": "members_count",
		"boards_count":  "boards_count",
		"tasks_count":   "tasks_count",
	}[f.OrderBy]
	if !ok {
		orderBySQL = "p.created_at"
	}
	orderSQL := "DESC"
	if f.Order == "ASC" {
		orderSQL = "ASC"
	}

	listArgs := append(append([]any{}, args...), limit, offset)
	listQuery := fmt.Sprintf(`
		SELECT
			p.id,
			p.name,
			p.description,
			p.created_at,
			p.deleted_at,
			(SELECT COUNT(*) FROM kanban_project_user pu WHERE pu.kanban_project_id = p.id) AS members_count,
			(SELECT COUNT(*) FROM kanban_board b WHERE b.kanban_project_id = p.id AND b.deleted_at IS NULL) AS boards_count,
			(
				SELECT COUNT(*)
				FROM kanban_card c
				JOIN kanban_column col ON col.id = c.column_id
				JOIN kanban_board b ON b.id = col.board_id
				WHERE b.kanban_project_id = p.id
				  AND b.deleted_at IS NULL
				  AND col.deleted_at IS NULL
				  AND c.is_archived = FALSE
				  AND c.deleted_at IS NULL
			) AS tasks_count,
			p.owner_id,
			u.login,
			u.lastname,
			u.firstname,
			u.patronymic,
			u.avatar_name
		FROM kanban_project p
		LEFT JOIN users u ON u.id = p.owner_id
		WHERE %s
		ORDER BY %s %s, p.id DESC
		LIMIT $%d OFFSET $%d`, where, orderBySQL, orderSQL, argN, argN+1)

	rows, err := r.Db.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, NormalizeError(err)
	}
	defer rows.Close()

	items := make([]model.ProjectListItem, 0)
	for rows.Next() {
		var item model.ProjectListItem
		var ownerID int64
		var ownerLogin, ownerLastname, ownerFirstname *string
		var ownerPatronymic, ownerAvatar *string
		if err := rows.Scan(
			&item.ID, &item.Name, &item.Description, &item.CreatedAt, &item.DeletedAt,
			&item.MembersCount, &item.BoardsCount, &item.TasksCount,
			&ownerID, &ownerLogin, &ownerLastname, &ownerFirstname, &ownerPatronymic, &ownerAvatar,
		); err != nil {
			return nil, err
		}
		if ownerLogin != nil {
			item.Owner = &model.User{
				ID:         ownerID,
				Login:      *ownerLogin,
				Lastname:   *ownerLastname,
				Firstname:  *ownerFirstname,
				Patronymic: ownerPatronymic,
				AvatarName: ownerAvatar,
			}
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.ProjectListPage{
		Items: items,
		Total: total,
		Page:  page,
		Limit: limit,
	}, nil
}

func (r *ProjectRepository) CreateProject(ctx context.Context, p *model.Project) (*model.Project, error) {
	query := `
		INSERT INTO kanban_project (name, description, owner_id, created_by_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, description, owner_id, created_by_id, created_at, updated_at, deleted_at`

	err := r.Db.QueryRow(ctx, query,
		p.Name, p.Description, p.OwnerID, p.CreatedByID,
	).Scan(
		&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedByID,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)

	if err != nil {
		return nil, NormalizeError(err)
	}
	return p, nil
}

func (r *ProjectRepository) GetProject(ctx context.Context, id int64) (*model.Project, error) {
	query := `
		SELECT id, name, description, owner_id, created_by_id, created_at, updated_at, deleted_at
		FROM kanban_project
		WHERE id = $1 AND deleted_at IS NULL`

	var p model.Project
	err := r.Db.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedByID,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)

	if err != nil {
		return nil, NormalizeError(err)
	}
	return &p, nil
}

func (r *ProjectRepository) UpdateProject(ctx context.Context, p *model.Project) (*model.Project, error) {
	query := `
		UPDATE kanban_project
		SET name = $1, description = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $3 AND deleted_at IS NULL
		RETURNING id, name, description, owner_id, created_by_id, created_at, updated_at, deleted_at`

	err := r.Db.QueryRow(ctx, query, p.Name, p.Description, p.ID).Scan(
		&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedByID,
		&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
	)

	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *ProjectRepository) DeleteProject(ctx context.Context, id int64) error {
	query := `
		UPDATE kanban_project
		SET deleted_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	_, err := r.Db.Exec(ctx, query, id)
	return err
}

func (r *ProjectRepository) GetNavProjectsForUser(ctx context.Context, userID int64) ([]model.NavProject, error) {
	query := `
		SELECT 
			p.id, 
			p.name, 
			p.description, 
			p.owner_id, 
			pu.role, 
			pu.folder_id, 
			pu.position,
			(
				SELECT b.id 
				FROM kanban_board b 
				WHERE b.kanban_project_id = p.id AND b.deleted_at IS NULL
				ORDER BY b.position ASC 
				LIMIT 1
			) as entry_board_id
		FROM kanban_project p
		JOIN kanban_project_user pu ON p.id = pu.kanban_project_id
		WHERE pu.user_id = $1 AND p.deleted_at IS NULL
		ORDER BY pu.position ASC`

	rows, err := r.Db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.NavProject
	for rows.Next() {
		var p model.NavProject
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.Role,
			&p.FolderID, &p.Position, &p.EntryBoardID,
		); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}

	return projects, rows.Err()
}
