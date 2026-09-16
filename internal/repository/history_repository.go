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
	LastByUser(ctx context.Context, projectID, userID int64) (*model.HistoryUndoEntry, error)
	HasForeignOverlap(ctx context.Context, projectID, afterID, userID int64, entityType string, entityID int64) (bool, error)
	ApplyUndo(ctx context.Context, entry model.HistoryUndoEntry) error
	DeleteEntry(ctx context.Context, id int64) error
}

type HistoryRepository struct {
	Db *pgxpool.Pool
}

func NewHistoryRepository(db *pgxpool.Pool) *HistoryRepository {
	return &HistoryRepository{Db: db}
}

func (r *HistoryRepository) Append(ctx context.Context, userID *int64, e model.HistoryWrite) error {
	payload, err := json.Marshal(model.HistoryPayload{
		Before: emptyToNil(e.Before),
		After:  emptyToNil(e.After),
	})
	if err != nil {
		return err
	}
	params := dbgen.CreateProjectHistoryEntryParams{
		ProjectID:   e.ProjectID,
		Action:      e.Action,
		EntityTitle: e.EntityTitle,
		EntityLink:  e.EntityLink,
		EntityType:  e.EntityType,
		EntityID:    e.EntityID,
		Payload:     payload,
	}
	if userID != nil {
		params.UserID = pgtype.Int8{Int64: *userID, Valid: true}
	}
	if e.CardID != 0 {
		params.CardID = pgtype.Int8{Int64: e.CardID, Valid: true}
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
			IsChild:     row.IsChild.Valid && row.IsChild.Bool,
			CreatedAt:   row.CreatedAt.Time.Format(time.RFC3339),
		}
		var payload model.HistoryPayload
		if len(row.Payload) > 0 && json.Unmarshal(row.Payload, &payload) == nil {
			item.Before = derefPayload(payload.Before)
			item.After = derefPayload(payload.After)
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

func (r *HistoryRepository) LastByUser(ctx context.Context, projectID, userID int64) (*model.HistoryUndoEntry, error) {
	row, err := dbgen.New(r.Db).GetLastProjectHistoryByUser(ctx, dbgen.GetLastProjectHistoryByUserParams{
		ProjectID: projectID,
		UserID:    pgtype.Int8{Int64: userID, Valid: true},
	})
	if err != nil {
		return nil, NormalizeError(err)
	}
	entry := model.HistoryUndoEntry{
		ID:          row.ID,
		Action:      row.Action,
		EntityType:  row.EntityType,
		EntityID:    row.EntityID,
		EntityTitle: row.EntityTitle,
		ProjectID:   row.ProjectID,
	}
	if row.CardID.Valid {
		entry.CardID = row.CardID.Int64
	}
	var payload model.HistoryPayload
	if len(row.Payload) > 0 && json.Unmarshal(row.Payload, &payload) == nil {
		entry.Before = derefPayload(payload.Before)
		entry.After = derefPayload(payload.After)
	}
	if row.CreatedAt.Valid {
		entry.CreatedAt = row.CreatedAt.Time
	}
	return &entry, nil
}

func (r *HistoryRepository) HasForeignOverlap(ctx context.Context, projectID, afterID, userID int64, entityType string, entityID int64) (bool, error) {
	return dbgen.New(r.Db).HasForeignNewerHistoryOverlap(ctx, dbgen.HasForeignNewerHistoryOverlapParams{
		ProjectID:  projectID,
		AfterID:    afterID,
		UserID:     pgtype.Int8{Int64: userID, Valid: true},
		EntityType: entityType,
		EntityID:   entityID,
	})
}

func (r *HistoryRepository) ApplyUndo(ctx context.Context, entry model.HistoryUndoEntry) error {
	return ExecTx(ctx, r.Db, func(q *dbgen.Queries) error {
		if err := applyHistoryUndo(ctx, q, entry); err != nil {
			return err
		}
		return q.DeleteProjectHistoryEntry(ctx, entry.ID)
	})
}

func (r *HistoryRepository) DeleteEntry(ctx context.Context, id int64) error {
	return dbgen.New(r.Db).DeleteProjectHistoryEntry(ctx, id)
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefPayload(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
