package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type BoardRepositoryInterface interface {
	GetBoardsByProject(ctx context.Context, projectID int64) ([]model.Board, error)
	CreateBoard(ctx context.Context, projectID int64, b *model.Board) (*model.Board, error)
	GetBoard(ctx context.Context, boardID int64) (*model.Board, error)
	UpdateBoard(ctx context.Context, b *model.Board) (*model.Board, error)
	DeleteBoard(ctx context.Context, boardID int64) error
	GetBoardArchive(ctx context.Context, boardID int64) ([]model.Card, error)
	HasColumnsByBoard(ctx context.Context, boardID int64) (bool, error)
}

type BoardRepository struct {
	Db *pgxpool.Pool
}

func NewBoardRepository(db *pgxpool.Pool) *BoardRepository {
	return &BoardRepository{
		Db: db,
	}
}

func (r *BoardRepository) GetBoardsByProject(ctx context.Context, projectID int64) ([]model.Board, error) {
	queries := dbgen.New(r.Db)
	dbBoards, err := queries.GetBoardsByProject(ctx, int32(projectID))
	if err != nil {
		return nil, err
	}

	var boards []model.Board
	for _, b := range dbBoards {
		boards = append(boards, model.Board{
			ID:              int64(b.ID),
			Title:           b.Title,
			Position:        b.Position,
			KanbanProjectID: int64(b.KanbanProjectID),
			CreatedByID:     int64(b.CreatedByID),
			CreatedAt:       b.CreatedAt.Time,
			UpdatedAt:       b.UpdatedAt.Time,
		})
	}
	return boards, nil
}

func (r *BoardRepository) CreateBoard(ctx context.Context, projectID int64, b *model.Board) (*model.Board, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.CreateBoard(ctx, dbgen.CreateBoardParams{
		Title:           b.Title,
		Position:        b.Position,
		KanbanProjectID: int32(projectID),
		CreatedByID:     int32(b.CreatedByID),
	})
	if err != nil {
		return nil, err
	}

	b.ID = int64(res.ID)
	b.CreatedAt = res.CreatedAt.Time
	b.UpdatedAt = res.UpdatedAt.Time
	return b, nil
}

func (r *BoardRepository) GetBoard(ctx context.Context, id int64) (*model.Board, error) {
	queries := dbgen.New(r.Db)
	b, err := queries.GetBoard(ctx, int32(id))
	if err != nil {
		return nil, err
	}

	return &model.Board{
		ID:              int64(b.ID),
		Title:           b.Title,
		Position:        b.Position,
		KanbanProjectID: int64(b.KanbanProjectID),
		CreatedByID:     int64(b.CreatedByID),
		CreatedAt:       b.CreatedAt.Time,
		UpdatedAt:       b.UpdatedAt.Time,
	}, nil
}

func (r *BoardRepository) UpdateBoard(ctx context.Context, b *model.Board) (*model.Board, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.UpdateBoard(ctx, dbgen.UpdateBoardParams{
		Title:    b.Title,
		Position: b.Position,
		ID:       int32(b.ID),
	})
	if err != nil {
		return nil, err
	}

	b.UpdatedAt = res.UpdatedAt.Time
	return b, nil
}

func (r *BoardRepository) DeleteBoard(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteBoard(ctx, int32(id))
}

func (r *BoardRepository) GetBoardArchive(ctx context.Context, boardID int64) ([]model.Card, error) {
	return []model.Card{}, nil
}

func (r *BoardRepository) HasColumnsByBoard(ctx context.Context, boardID int64) (bool, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.HasColumnsByBoard(ctx, int32(boardID))
	if err != nil {
		return false, err
	}
	return res, nil
}
