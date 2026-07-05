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
	queries := dbgen.New(r.Db)
	dbActivities, err := queries.GetActivitiesByCard(ctx, int32(cardID))
	if err != nil {
		return nil, err
	}
	
	var activities []model.Activity
	for _, a := range dbActivities {
		act := model.Activity{
			ID:        int64(a.ID),
			CardID:    int64(a.CardID),
			Type:      a.Type,
			CreatedAt: a.CreatedAt.Time,
		}
		if a.UserID.Valid {
			uid := int64(a.UserID.Int32)
			act.UserID = &uid
			
			var nameParts string
			if a.Firstname.Valid {
				nameParts += a.Firstname.String
			}
			if a.Lastname.Valid {
				if nameParts != "" {
					nameParts += " "
				}
				nameParts += a.Lastname.String
			}
			if nameParts != "" {
				act.UserName = &nameParts
			}
		}
		if a.OldValue.Valid {
			act.OldValue = &a.OldValue.String
		}
		if a.NewValue.Valid {
			act.NewValue = &a.NewValue.String
		}
		activities = append(activities, act)
	}
	return activities, nil
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
