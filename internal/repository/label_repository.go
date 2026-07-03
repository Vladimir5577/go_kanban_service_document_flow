package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type LabelRepositoryInterface interface {
	GetLabels(ctx context.Context, boardID int64) ([]model.Label, error)
	CreateLabel(ctx context.Context, boardID int64, l *model.Label) (*model.Label, error)
	DeleteLabel(ctx context.Context, labelID int64) error
	ToggleLabel(ctx context.Context, cardID int64, labelID int64) error
}

type LabelRepository struct {
	Db *pgxpool.Pool
}

func NewLabelRepository(db *pgxpool.Pool) *LabelRepository {
	return &LabelRepository{
		Db: db,
	}
}

func (r *LabelRepository) GetLabels(ctx context.Context, boardID int64) ([]model.Label, error) {
	queries := dbgen.New(r.Db)
	dbLabels, err := queries.GetLabelsByBoard(ctx, int32(boardID))
	if err != nil {
		return nil, err
	}

	var labels []model.Label
	for _, l := range dbLabels {
		labels = append(labels, model.Label{
			ID:      int64(l.ID),
			Name:    l.Name,
			Color:   l.Color,
			BoardID: int64(l.BoardID),
		})
	}
	return labels, nil
}

func (r *LabelRepository) CreateLabel(ctx context.Context, boardID int64, l *model.Label) (*model.Label, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.CreateLabel(ctx, dbgen.CreateLabelParams{
		Name:    l.Name,
		Color:   l.Color,
		BoardID: int32(boardID),
	})
	if err != nil {
		return nil, err
	}

	l.ID = int64(res.ID)
	return l, nil
}

func (r *LabelRepository) DeleteLabel(ctx context.Context, labelID int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteLabel(ctx, int32(labelID))
}

func (r *LabelRepository) ToggleLabel(ctx context.Context, cardID int64, labelID int64) error {
	return nil
}
