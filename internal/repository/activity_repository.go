package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type ActivityRepositoryInterface interface {
	GetActivities(ctx context.Context, cardID int64) ([]model.Activity, error)
	LogActivity(ctx context.Context, cardID int64, authorID *int64, action string, oldValue, newValue *string) error
}

type ActivityRepository struct {
	Db *pgxpool.Pool
}

func NewActivityRepository(db *pgxpool.Pool) *ActivityRepository {
	return &ActivityRepository{
		Db: db,
	}
}

func (r *ActivityRepository) GetActivities(ctx context.Context, cardID int64) ([]model.Activity, error) {
	return []model.Activity{}, nil
}

func (r *ActivityRepository) LogActivity(ctx context.Context, cardID int64, authorID *int64, action string, oldValue, newValue *string) error {
	queries := dbgen.New(r.Db)

	params := dbgen.CreateActivityParams{
		Type:   action,
		CardID: int32(cardID),
	}
	if authorID != nil {
		params.UserID = pgtype.Int4{Int32: int32(*authorID), Valid: true}
	}
	if oldValue != nil {
		params.OldValue = pgtype.Text{String: *oldValue, Valid: true}
	}
	if newValue != nil {
		params.NewValue = pgtype.Text{String: *newValue, Valid: true}
	}

	_, err := queries.CreateActivity(ctx, params)
	return err
}
