package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type SubtaskRepositoryInterface interface {
	GetSubtask(ctx context.Context, id int64) (*model.Subtask, error)
	GetSubtasks(ctx context.Context, cardID int64) ([]model.Subtask, error)
	GetChecklistCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]model.ChecklistCount, error)
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
	dbSubtasks, err := queries.GetSubtasksByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}

	var subtasks []model.Subtask
	for _, s := range dbSubtasks {
		st := model.Subtask{
			ID:       s.ID,
			Title:    s.Title,
			Status:   s.Status,
			Position: s.Position,
			CardID:   s.CardID,
		}
		if s.UserID.Valid {
			uid := s.UserID.Int64
			st.UserID = &uid
		}
		subtasks = append(subtasks, st)
	}
	return subtasks, nil
}

func (r *SubtaskRepository) GetChecklistCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]model.ChecklistCount, error) {
	if len(cardIDs) == 0 {
		return make(map[int64]model.ChecklistCount), nil
	}

	queries := dbgen.New(r.Db)
	rows, err := queries.GetSubtaskCountsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]model.ChecklistCount)
	for _, row := range rows {
		result[row.CardID] = model.ChecklistCount{
			Total: int(row.Total),
			Done:  int(row.Done),
		}
	}

	return result, nil
}

func (r *SubtaskRepository) GetSubtask(ctx context.Context, id int64) (*model.Subtask, error) {
	queries := dbgen.New(r.Db)
	dbSubtask, err := queries.GetSubtask(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}
	s := &model.Subtask{
		ID:       dbSubtask.ID,
		Title:    dbSubtask.Title,
		Status:   dbSubtask.Status,
		Position: dbSubtask.Position,
		CardID:   dbSubtask.CardID,
	}
	if dbSubtask.UserID.Valid {
		uid := dbSubtask.UserID.Int64
		s.UserID = &uid
	}
	return s, nil
}

func (r *SubtaskRepository) CreateSubtask(ctx context.Context, cardID int64, s *model.Subtask) (*model.Subtask, error) {
	queries := dbgen.New(r.Db)

	var userID pgtype.Int8
	if s.UserID != nil {
		userID = pgtype.Int8{Int64: *s.UserID, Valid: true}
	}

	res, err := queries.CreateSubtask(ctx, dbgen.CreateSubtaskParams{
		Title:    s.Title,
		Status:   s.Status,
		Position: s.Position,
		CardID:   cardID,
		UserID:   userID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	s.ID = res.ID
	return s, nil
}

func (r *SubtaskRepository) UpdateSubtask(ctx context.Context, subtaskID int64, s *model.Subtask) (*model.Subtask, error) {
	queries := dbgen.New(r.Db)

	var userID pgtype.Int8
	if s.UserID != nil {
		userID = pgtype.Int8{Int64: *s.UserID, Valid: true}
	}

	_, err := queries.UpdateSubtask(ctx, dbgen.UpdateSubtaskParams{
		Title:    s.Title,
		Status:   s.Status,
		Position: s.Position,
		UserID:   userID,
		ID:       subtaskID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}
	return s, nil
}

func (r *SubtaskRepository) DeleteSubtask(ctx context.Context, subtaskID int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteSubtask(ctx, subtaskID)
}
