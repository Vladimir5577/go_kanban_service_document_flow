package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

// CommentAttachmentRepositoryInterface — файлы комментариев (kanban_card_comment_attachment).
type CommentAttachmentRepositoryInterface interface {
	GetByCard(ctx context.Context, cardID int64) ([]model.CommentAttachment, error)
	GetByComment(ctx context.Context, commentID int64) ([]model.CommentAttachment, error)
	Get(ctx context.Context, id int64) (*model.CommentAttachment, error)
	Create(ctx context.Context, a *model.CommentAttachment) (*model.CommentAttachment, error)
	Delete(ctx context.Context, id int64) error
}

type CommentAttachmentRepository struct {
	Db *pgxpool.Pool
}

func NewCommentAttachmentRepository(db *pgxpool.Pool) *CommentAttachmentRepository {
	return &CommentAttachmentRepository{Db: db}
}

func mapCommentAttachment(a dbgen.KanbanCardCommentAttachment) model.CommentAttachment {
	return model.CommentAttachment{
		ID:          a.ID,
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		CommentID:   a.CommentID,
		CreatedAt:   a.CreatedAt.Time,
	}
}

func mapCommentAttachments(rows []dbgen.KanbanCardCommentAttachment) []model.CommentAttachment {
	attachments := make([]model.CommentAttachment, 0, len(rows))
	for _, a := range rows {
		attachments = append(attachments, mapCommentAttachment(a))
	}
	return attachments
}

// GetByCard — файлы всех живых комментариев карточки.
func (r *CommentAttachmentRepository) GetByCard(ctx context.Context, cardID int64) ([]model.CommentAttachment, error) {
	rows, err := dbgen.New(r.Db).GetCommentAttachmentsByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}
	return mapCommentAttachments(rows), nil
}

func (r *CommentAttachmentRepository) GetByComment(ctx context.Context, commentID int64) ([]model.CommentAttachment, error) {
	rows, err := dbgen.New(r.Db).GetCommentAttachmentsByComment(ctx, commentID)
	if err != nil {
		return nil, err
	}
	return mapCommentAttachments(rows), nil
}

func (r *CommentAttachmentRepository) Get(ctx context.Context, id int64) (*model.CommentAttachment, error) {
	a, err := dbgen.New(r.Db).GetCommentAttachment(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}
	att := mapCommentAttachment(a)
	return &att, nil
}

func (r *CommentAttachmentRepository) Create(ctx context.Context, a *model.CommentAttachment) (*model.CommentAttachment, error) {
	res, err := dbgen.New(r.Db).CreateCommentAttachment(ctx, dbgen.CreateCommentAttachmentParams{
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		CommentID:   a.CommentID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	a.ID = res.ID
	a.CreatedAt = res.CreatedAt.Time
	return a, nil
}

func (r *CommentAttachmentRepository) Delete(ctx context.Context, id int64) error {
	return dbgen.New(r.Db).DeleteCommentAttachment(ctx, id)
}
