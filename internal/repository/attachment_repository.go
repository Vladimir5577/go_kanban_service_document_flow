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
	queries := dbgen.New(r.Db)
	dbAtt, err := queries.GetAttachmentsByCard(ctx, dbgen.GetAttachmentsByCardParams{
		CardID:  int32(cardID),
		Context: contextStr,
	})
	if err != nil {
		return nil, err
	}

	var attachments []model.Attachment
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

func (r *AttachmentRepository) GetAttachment(ctx context.Context, id int64) (*model.Attachment, error) {
	queries := dbgen.New(r.Db)
	a, err := queries.GetAttachment(ctx, int32(id))
	if err != nil {
		return nil, err
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
		return nil, err
	}

	a.ID = int64(res.ID)
	a.CreatedAt = res.CreatedAt.Time
	return a, nil
}

func (r *AttachmentRepository) DeleteAttachment(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteAttachment(ctx, int32(id))
}
