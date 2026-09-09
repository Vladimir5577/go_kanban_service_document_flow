package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type HistoryRepositoryInterface interface {
	Append(ctx context.Context, userID *int64, e model.HistoryWrite) error
	List(ctx context.Context, projectID, cardID, userID, cursor int64, title string, limit int32) ([]model.HistoryListItem, error)
	LastByUser(ctx context.Context, projectID, userID int64) (*dbgen.KanbanProjectHistory, error)
	HasForeignOverlap(ctx context.Context, projectID, afterID, userID int64, keys []string) (bool, error)
	ApplyUndo(ctx context.Context, entryID int64, steps []model.UndoStep) error
}

type HistoryRepository struct {
	Db *pgxpool.Pool
}

func NewHistoryRepository(db *pgxpool.Pool) *HistoryRepository {
	return &HistoryRepository{Db: db}
}

func (r *HistoryRepository) Append(ctx context.Context, userID *int64, e model.HistoryWrite) error {
	payload, err := json.Marshal(model.HistoryPayload{Steps: e.Undo, Before: e.Before, After: e.After})
	if err != nil {
		return err
	}
	params := dbgen.CreateProjectHistoryEntryParams{
		ProjectID:   e.ProjectID,
		Action:      e.Action,
		EntityTitle: e.EntityTitle,
		EntityLink:  e.EntityLink,
		EntityKeys:  e.EntityKeys,
		Payload:     payload,
	}
	if userID != nil {
		params.UserID = pgtype.Int8{Int64: *userID, Valid: true}
	}
	_, err = dbgen.New(r.Db).CreateProjectHistoryEntry(ctx, params)
	return err
}

func (r *HistoryRepository) List(ctx context.Context, projectID, cardID, userID, cursor int64, title string, limit int32) ([]model.HistoryListItem, error) {
	rows, err := dbgen.New(r.Db).ListProjectHistory(ctx, dbgen.ListProjectHistoryParams{
		ProjectID:  projectID,
		CardID:     cardID,
		UserID:     userID,
		TitleQuery: title,
		Cursor:     cursor,
		PageLimit:  limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]model.HistoryListItem, 0, len(rows))
	for _, row := range rows {
		item := model.HistoryListItem{
			ID:          row.ID,
			Action:      row.Action,
			EntityTitle: row.EntityTitle,
			EntityLink:  row.EntityLink,
			CreatedAt:   row.CreatedAt.Time.Format(time.RFC3339),
		}
		var payload model.HistoryPayload
		if len(row.Payload) > 0 && json.Unmarshal(row.Payload, &payload) == nil {
			item.Before = payload.Before
			item.After = payload.After
		}
		if row.UserID.Valid {
			id := row.UserID.Int64
			item.UserID = &id
		}
		item.UserName = joinPersonName(row.Lastname, row.Firstname, row.Patronymic)
		items = append(items, item)
	}
	return items, nil
}

func (r *HistoryRepository) LastByUser(ctx context.Context, projectID, userID int64) (*dbgen.KanbanProjectHistory, error) {
	row, err := dbgen.New(r.Db).GetLastProjectHistoryByUser(ctx, dbgen.GetLastProjectHistoryByUserParams{
		ProjectID: projectID,
		UserID:    pgtype.Int8{Int64: userID, Valid: true},
	})
	if err != nil {
		return nil, NormalizeError(err)
	}
	return &row, nil
}

func (r *HistoryRepository) HasForeignOverlap(ctx context.Context, projectID, afterID, userID int64, keys []string) (bool, error) {
	return dbgen.New(r.Db).HasForeignNewerHistoryOverlap(ctx, dbgen.HasForeignNewerHistoryOverlapParams{
		ProjectID:  projectID,
		AfterID:    afterID,
		UserID:     pgtype.Int8{Int64: userID, Valid: true},
		EntityKeys: keys,
	})
}

func (r *HistoryRepository) ApplyUndo(ctx context.Context, entryID int64, steps []model.UndoStep) error {
	return ExecTx(ctx, r.Db, func(q *dbgen.Queries) error {
		for _, step := range steps {
			if err := applyUndoStep(ctx, q, step); err != nil {
				return err
			}
		}
		return q.DeleteProjectHistoryEntry(ctx, entryID)
	})
}

func joinPersonName(last, first pgtype.Text, patronymic pgtype.Text) string {
	parts := make([]string, 0, 3)
	if last.Valid && last.String != "" {
		parts = append(parts, last.String)
	}
	if first.Valid && first.String != "" {
		parts = append(parts, first.String)
	}
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	if len(parts) > 1 {
		out += " " + parts[1]
	}
	return out
}
