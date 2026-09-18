package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type CommentReadRepositoryInterface interface {
	MarkRead(ctx context.Context, cardID, userID, upToCommentID int64) error
	GetReaders(ctx context.Context, cardID, commentID int64) ([]model.CommentRead, error)
	GetMark(ctx context.Context, cardID, userID int64) (int64, error)
}

type CommentReadRepository struct {
	Db *pgxpool.Pool
}

func NewCommentReadRepository(db *pgxpool.Pool) *CommentReadRepository {
	return &CommentReadRepository{
		Db: db,
	}
}

// MarkRead двигает знак пользователя. Чужой, удалённый или меньший id молча
// ничего не меняет — вся проверка в самом запросе.
func (r *CommentReadRepository) MarkRead(ctx context.Context, cardID, userID, upToCommentID int64) error {
	queries := dbgen.New(r.Db)
	return queries.MarkCommentsRead(ctx, dbgen.MarkCommentsReadParams{
		CardID:        cardID,
		UserID:        userID,
		UpToCommentID: upToCommentID,
	})
}

func (r *CommentReadRepository) GetReaders(ctx context.Context, cardID, commentID int64) ([]model.CommentRead, error) {
	queries := dbgen.New(r.Db)
	rows, err := queries.GetCommentReaders(ctx, dbgen.GetCommentReadersParams{
		CardID:        cardID,
		UpToCommentID: commentID,
	})
	if err != nil {
		return nil, err
	}

	readers := make([]model.CommentRead, 0, len(rows))
	for _, row := range rows {
		readers = append(readers, model.CommentRead{
			UserID: row.UserID,
			ReadAt: row.ReadAt.Time,
		})
	}
	return readers, nil
}

// GetMark — докуда пользователь дочитал карточку. Ноль означает «ни разу».
func (r *CommentReadRepository) GetMark(ctx context.Context, cardID, userID int64) (int64, error) {
	queries := dbgen.New(r.Db)
	return queries.GetCardReadMark(ctx, dbgen.GetCardReadMarkParams{
		CardID: cardID,
		UserID: userID,
	})
}
