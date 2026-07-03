package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type SubtaskRepositoryInterface interface {
	GetSubtasks(ctx context.Context, cardID int64) ([]model.Subtask, error)
	CreateSubtask(ctx context.Context, cardID int64, s *model.Subtask) (*model.Subtask, error)
	UpdateSubtask(ctx context.Context, subtaskID int64, s *model.Subtask) (*model.Subtask, error)
	DeleteSubtask(ctx context.Context, subtaskID int64) error
}

type SubtaskRepository struct {
	Db *pgxpool.Pool
}

func NewSubtaskRepository(db *pgxpool.Pool) *SubtaskRepository {
	return &SubtaskRepository{
		Db: db,
	}
}

func (r *SubtaskRepository) GetSubtasks(ctx context.Context, cardID int64) ([]model.Subtask, error) {
	queries := dbgen.New(r.Db)
	dbSubtasks, err := queries.GetSubtasksByCard(ctx, int32(cardID))
	if err != nil {
		return nil, err
	}

	var subtasks []model.Subtask
	for _, s := range dbSubtasks {
		subtasks = append(subtasks, model.Subtask{
			ID:       int64(s.ID),
			Title:    s.Title,
			Status:   s.Status,
			Position: s.Position,
			CardID:   int64(s.CardID),
		})
	}
	return subtasks, nil
}

func (r *SubtaskRepository) CreateSubtask(ctx context.Context, cardID int64, s *model.Subtask) (*model.Subtask, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.CreateSubtask(ctx, dbgen.CreateSubtaskParams{
		Title:    s.Title,
		Status:   s.Status,
		Position: s.Position,
		CardID:   int32(cardID),
		// TODO: add UserID mapping when it's added to model
	})
	if err != nil {
		return nil, err
	}

	s.ID = int64(res.ID)
	return s, nil
}

func (r *SubtaskRepository) UpdateSubtask(ctx context.Context, subtaskID int64, s *model.Subtask) (*model.Subtask, error) {
	queries := dbgen.New(r.Db)
	_, err := queries.UpdateSubtask(ctx, dbgen.UpdateSubtaskParams{
		Title:    s.Title,
		Status:   s.Status,
		Position: s.Position,
		ID:       int32(subtaskID),
		// TODO: add UserID mapping when it's added to model
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *SubtaskRepository) DeleteSubtask(ctx context.Context, subtaskID int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteSubtask(ctx, int32(subtaskID))
}
