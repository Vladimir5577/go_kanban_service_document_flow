package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
)

type ProjectRepositoryInterface interface {
	GetAllProjects(ctx context.Context) ([]model.Project, error)
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

func (r *ProjectRepository) GetAllProjects(ctx context.Context) ([]model.Project, error) {
	query := `
		SELECT id, name, description, owner_id, created_by_id, created_at, updated_at, deleted_at
		FROM kanban_project
		WHERE deleted_at IS NULL
		ORDER BY id DESC`

	rows, err := r.Db.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []model.Project
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description, &p.OwnerID, &p.CreatedByID,
			&p.CreatedAt, &p.UpdatedAt, &p.DeletedAt,
		); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}

	return projects, rows.Err()
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
		return nil, err
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
		return nil, err
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
