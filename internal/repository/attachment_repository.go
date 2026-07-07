package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

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
	Db *pgxpool.Pool
}

func NewAttachmentRepository(db *pgxpool.Pool) *AttachmentRepository {
	return &AttachmentRepository{
		Db: db,
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
			var authorID *int32
			err := rows.Scan(&a.ID, &a.Filename, &a.StorageKey, &a.ContentType, &a.SizeBytes, &a.Context, &a.CardID, &authorID, &a.CreatedAt)
			if err != nil {
				return nil, err
			}
			if authorID != nil {
				v := int64(*authorID)
				a.AuthorID = &v
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
		CardID:  int32(cardID),
		Context: contextStr,
	})
	if err != nil {
		return nil, err
	}

	for _, a := range dbAtt {
		att := model.Attachment{
			ID:          int64(a.ID),
			Filename:    a.Filename,
			StorageKey:  a.StorageKey,
			ContentType: a.ContentType,
			SizeBytes:   int64(a.SizeBytes),
			Context:     a.Context,
			CardID:      int64(a.CardID),
			CreatedAt:   a.CreatedAt.Time,
		}
		if a.AuthorID.Valid {
			v := int64(a.AuthorID.Int32)
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

	cardIDs32 := make([]int32, len(cardIDs))
	for i, id := range cardIDs {
		cardIDs32[i] = int32(id)
	}

	queries := dbgen.New(r.Db)
	rows, err := queries.GetChatAttachmentCountsByCardIDs(ctx, cardIDs32)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]int)
	for _, row := range rows {
		cardID := int64(row.CardID)
		result[cardID] = int(row.Count)
	}

	return result, nil
}

func (r *AttachmentRepository) GetAttachment(ctx context.Context, id int64) (*model.Attachment, error) {
	queries := dbgen.New(r.Db)
	a, err := queries.GetAttachment(ctx, int32(id))
	if err != nil {
		return nil, NormalizeError(err)
	}

	att := &model.Attachment{
		ID:          int64(a.ID),
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   int64(a.SizeBytes),
		Context:     a.Context,
		CardID:      int64(a.CardID),
		CreatedAt:   a.CreatedAt.Time,
	}
	if a.AuthorID.Valid {
		v := int64(a.AuthorID.Int32)
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
		SizeBytes:   int32(a.SizeBytes),
		Context:     a.Context,
		CardID:      int32(cardID),
	}
	if a.AuthorID != nil {
		params.AuthorID = pgtype.Int4{Int32: int32(*a.AuthorID), Valid: true}
	}

	res, err := queries.CreateAttachment(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	a.ID = int64(res.ID)
	a.CreatedAt = res.CreatedAt.Time
	return a, nil
}

func (r *AttachmentRepository) DeleteAttachment(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteAttachment(ctx, int32(id))
}
