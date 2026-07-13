package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type AttachmentRepositoryInterface interface {
	GetAttachmentsByCard(ctx context.Context, cardID int64, contextStr string) ([]model.Attachment, error)
	GetChatCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]int, error)
	GetAttachment(ctx context.Context, id int64) (*model.Attachment, error)
	CreateAttachment(ctx context.Context, cardID int64, a *model.Attachment) (*model.Attachment, error)
	DeleteAttachment(ctx context.Context, id int64) error
}

type AttachmentRepository struct {
	Db    *pgxpool.Pool
	clock helper.Clock
}

func NewAttachmentRepository(db *pgxpool.Pool, clk helper.Clock) *AttachmentRepository {
	return &AttachmentRepository{
		Db:    db,
		clock: clk,
	}
}

func (r *AttachmentRepository) GetAttachmentsByCard(ctx context.Context, cardID int64, contextStr string) ([]model.Attachment, error) {
	var attachments []model.Attachment

	if contextStr == "" {
		rows, err := r.Db.Query(ctx, "SELECT id, filename, storage_key, content_type, size_bytes, context, card_id, author_id, created_at FROM kanban_attachment WHERE card_id = $1 ORDER BY created_at ASC", cardID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		for rows.Next() {
			var a model.Attachment
			var authorID *int64
			err := rows.Scan(&a.ID, &a.Filename, &a.StorageKey, &a.ContentType, &a.SizeBytes, &a.Context, &a.CardID, &authorID, &a.CreatedAt)
			if err != nil {
				return nil, err
			}
			if authorID != nil {
				a.AuthorID = authorID
			}
			attachments = append(attachments, a)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return attachments, nil
	}

	queries := dbgen.New(r.Db)
	dbAtt, err := queries.GetAttachmentsByCard(ctx, dbgen.GetAttachmentsByCardParams{
		CardID:  cardID,
		Context: contextStr,
	})
	if err != nil {
		return nil, err
	}

	for _, a := range dbAtt {
		att := model.Attachment{
			ID:          a.ID,
			Filename:    a.Filename,
			StorageKey:  a.StorageKey,
			ContentType: a.ContentType,
			SizeBytes:   a.SizeBytes,
			Context:     a.Context,
			CardID:      a.CardID,
			CreatedAt:   r.clock.FromDB(a.CreatedAt.Time),
		}
		if a.AuthorID.Valid {
			v := a.AuthorID.Int64
			att.AuthorID = &v
		}
		attachments = append(attachments, att)
	}
	return attachments, nil
}

func (r *AttachmentRepository) GetChatCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]int, error) {
	if len(cardIDs) == 0 {
		return make(map[int64]int), nil
	}

	queries := dbgen.New(r.Db)
	rows, err := queries.GetChatAttachmentCountsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]int)
	for _, row := range rows {
		result[row.CardID] = int(row.Count)
	}

	return result, nil
}

func (r *AttachmentRepository) GetAttachment(ctx context.Context, id int64) (*model.Attachment, error) {
	queries := dbgen.New(r.Db)
	a, err := queries.GetAttachment(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}

	att := &model.Attachment{
		ID:          a.ID,
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		Context:     a.Context,
		CardID:      a.CardID,
		CreatedAt:   a.CreatedAt.Time,
	}
	if a.AuthorID.Valid {
		v := a.AuthorID.Int64
		att.AuthorID = &v
	}
	return att, nil
}

func (r *AttachmentRepository) CreateAttachment(ctx context.Context, cardID int64, a *model.Attachment) (*model.Attachment, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.CreateAttachmentParams{
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		Context:     a.Context,
		CardID:      cardID,
	}
	if a.AuthorID != nil {
		params.AuthorID = pgtype.Int8{Int64: *a.AuthorID, Valid: true}
	}

	res, err := queries.CreateAttachment(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	a.ID = res.ID
	a.CreatedAt = r.clock.FromDB(res.CreatedAt.Time)
	return a, nil
}

func (r *AttachmentRepository) DeleteAttachment(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteAttachment(ctx, id)
}
