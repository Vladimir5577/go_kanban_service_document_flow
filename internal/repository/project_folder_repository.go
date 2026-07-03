package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
)

type ProjectFolderRepositoryInterface interface {
	GetProjectFolders(ctx context.Context) ([]model.ProjectUserFolder, error)
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
