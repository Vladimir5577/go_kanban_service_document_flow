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
	dbCols, err := queries.GetColumnsByBoard(ctx, boardID)
	if err != nil {
		return nil, err
	}

	var cols []model.Column
	for _, c := range dbCols {
		cols = append(cols, model.Column{
			ID:          c.ID,
			Title:       c.Title,
			HeaderColor: c.HeaderColor,
			Position:    c.Position,
			BoardID:     c.BoardID,
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
		BoardID:     boardID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.ID = res.ID
	c.Title = res.Title
	c.HeaderColor = res.HeaderColor
	c.Position = res.Position
	c.BoardID = res.BoardID
	return c, nil
}

func (r *ColumnRepository) GetColumn(ctx context.Context, id int64) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetColumn(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}

	return &model.Column{
		ID:          c.ID,
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		BoardID:     c.BoardID,
	}, nil
}

func (r *ColumnRepository) UpdateColumn(ctx context.Context, c *model.Column) (*model.Column, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.UpdateColumn(ctx, dbgen.UpdateColumnParams{
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		ID:          c.ID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.ID = res.ID
	c.Title = res.Title
	c.HeaderColor = res.HeaderColor
	c.Position = res.Position
	c.BoardID = res.BoardID
	return c, nil
}

func (r *ColumnRepository) DeleteColumn(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteColumn(ctx, id)
}

func (r *ColumnRepository) HasCardsByColumn(ctx context.Context, columnID int64) (bool, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.HasCardsByColumn(ctx, columnID)
	if err != nil {
		return false, err
	}
	return res, nil
}
