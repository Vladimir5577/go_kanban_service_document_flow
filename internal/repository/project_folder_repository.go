package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
)

type ProjectFolderRepositoryInterface interface {
	GetProjectFolders(ctx context.Context) ([]model.ProjectUserFolder, error)
	GetProjectFolder(ctx context.Context, id int64) (*model.ProjectUserFolder, error)
}

type ProjectFolderRepository struct {
	Db *pgxpool.Pool
}

func NewProjectFolderRepository(db *pgxpool.Pool) *ProjectFolderRepository {
	return &ProjectFolderRepository{
		Db: db,
	}
}

func (r *ProjectFolderRepository) GetProjectFolders(ctx context.Context) ([]model.ProjectUserFolder, error) {
	return []model.ProjectUserFolder{}, nil
}

func (r *ProjectFolderRepository) GetProjectFolder(ctx context.Context, id int64) (*model.ProjectUserFolder, error) {
	query := `
		SELECT id, name, user_id, position, created_at, updated_at
		FROM kanban_project_user_folder
		WHERE id = $1`

	var folder model.ProjectUserFolder
	err := r.Db.QueryRow(ctx, query, id).Scan(
		&folder.ID,
		&folder.Name,
		&folder.UserID,
		&folder.Position,
		&folder.CreatedAt,
		&folder.UpdatedAt,
	)
	if err != nil {
		return nil, NormalizeError(err)
	}
	return &folder, nil
}
