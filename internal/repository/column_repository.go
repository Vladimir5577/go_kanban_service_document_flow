package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type ColumnRepositoryInterface interface {
	CreateColumn(ctx context.Context, boardID int64, c *model.Column) (*model.Column, error)
	UpdateColumn(ctx context.Context, c *model.Column) (*model.Column, error)
	DeleteColumn(ctx context.Context, id int64) error
	GetColumn(ctx context.Context, id int64) (*model.Column, error)
	GetColumnsByBoard(ctx context.Context, boardID int64) ([]model.Column, error)
	HasCardsByColumn(ctx context.Context, columnID int64) (bool, error)
}

type ColumnRepository struct {
	Db *pgxpool.Pool
}

func NewColumnRepository(db *pgxpool.Pool) *ColumnRepository {
	return &ColumnRepository{
		Db: db,
	}
}

func (r *ColumnRepository) GetColumnsByBoard(ctx context.Context, boardID int64) ([]model.Column, error) {
	queries := dbgen.New(r.Db)
	dbCols, err := queries.GetColumnsByBoard(ctx, int32(boardID))
	if err != nil {
		return nil, err
	}

	var cols []model.Column
	for _, c := range dbCols {
		cols = append(cols, model.Column{
			ID:          int64(c.ID),
			Title:       c.Title,
			HeaderColor: c.HeaderColor,
			Position:    c.Position,
			BoardID:     int64(c.BoardID),
		})
	}
	return cols, nil
}

func (r *ColumnRepository) CreateColumn(ctx context.Context, boardID int64, c *model.Column) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.CreateColumn(ctx, dbgen.CreateColumnParams{
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		BoardID:     int32(boardID),
	})
	if err != nil {
		return nil, err
	}

	c.ID = int64(res.ID)
	return c, nil
}

func (r *ColumnRepository) GetColumn(ctx context.Context, id int64) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetColumn(ctx, int32(id))
	if err != nil {
		return nil, err
	}

	return &model.Column{
		ID:          int64(c.ID),
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		BoardID:     int64(c.BoardID),
	}, nil
}

func (r *ColumnRepository) UpdateColumn(ctx context.Context, c *model.Column) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	_, err := queries.UpdateColumn(ctx, dbgen.UpdateColumnParams{
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		ID:          int32(c.ID),
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ColumnRepository) DeleteColumn(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteColumn(ctx, int32(id))
}

func (r *ColumnRepository) HasCardsByColumn(ctx context.Context, columnID int64) (bool, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.HasCardsByColumn(ctx, int32(columnID))
	if err != nil {
		return false, err
	}
	return res, nil
}
