package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type AttachmentRepositoryInterface interface {
	GetAttachmentsByCard(ctx context.Context, cardID int64) ([]model.Attachment, error)
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

func mapAttachment(a dbgen.KanbanAttachment) model.Attachment {
	att := model.Attachment{
		ID:          a.ID,
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		CardID:      a.CardID,
		CreatedAt:   a.CreatedAt.Time,
	}
	if a.AuthorID.Valid {
		v := a.AuthorID.Int64
		att.AuthorID = &v
	}
	return att
}

func (r *AttachmentRepository) GetAttachmentsByCard(ctx context.Context, cardID int64) ([]model.Attachment, error) {
	rows, err := dbgen.New(r.Db).GetAttachmentsByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	attachments := make([]model.Attachment, 0, len(rows))
	for _, a := range rows {
		attachments = append(attachments, mapAttachment(a))
	}
	return attachments, nil
}

func (r *AttachmentRepository) GetAttachment(ctx context.Context, id int64) (*model.Attachment, error) {
	a, err := dbgen.New(r.Db).GetAttachment(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}
	att := mapAttachment(a)
	return &att, nil
}

func (r *AttachmentRepository) CreateAttachment(ctx context.Context, cardID int64, a *model.Attachment) (*model.Attachment, error) {
	params := dbgen.CreateAttachmentParams{
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		CardID:      cardID,
	}
	if a.AuthorID != nil {
		params.AuthorID = pgtype.Int8{Int64: *a.AuthorID, Valid: true}
	}

	res, err := dbgen.New(r.Db).CreateAttachment(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	a.ID = res.ID
	a.CreatedAt = res.CreatedAt.Time
	return a, nil
}

func (r *AttachmentRepository) DeleteAttachment(ctx context.Context, id int64) error {
	return dbgen.New(r.Db).DeleteAttachment(ctx, id)
}
