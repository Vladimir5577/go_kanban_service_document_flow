package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

type CommentRepositoryInterface interface {
	GetComments(ctx context.Context, cardID int64) ([]model.Comment, error)
	GetCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]int, error)
	GetComment(ctx context.Context, id int64) (*model.Comment, error)
	CreateComment(ctx context.Context, cardID int64, c *model.Comment) (*model.Comment, error)
	UpdateComment(ctx context.Context, c *model.Comment) (*model.Comment, error)
	DeleteComment(ctx context.Context, commentID int64) error
}

type CommentRepository struct {
	Db *pgxpool.Pool
}

func NewCommentRepository(db *pgxpool.Pool) *CommentRepository {
	return &CommentRepository{
		Db: db,
	}
}

func (r *CommentRepository) GetComments(ctx context.Context, cardID int64) ([]model.Comment, error) {
	queries := dbgen.New(r.Db)
	dbComments, err := queries.GetCommentsByCard(ctx, int32(cardID))
	if err != nil {
		return nil, err
	}

	var comments []model.Comment
	for _, c := range dbComments {
		comment := model.Comment{
			ID:        int64(c.ID),
			Body:      c.Body,
			CardID:    int64(c.CardID),
			CreatedAt: c.CreatedAt.Time,
		}
		if c.UpdatedAt.Valid {
			comment.UpdatedAt = &c.UpdatedAt.Time
		}
		comment.AuthorID = int64(c.AuthorID)
		comments = append(comments, comment)
	}
	return comments, nil
}

func (r *CommentRepository) GetCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]int, error) {
	if len(cardIDs) == 0 {
		return make(map[int64]int), nil
	}

	cardIDs32 := make([]int32, len(cardIDs))
	for i, id := range cardIDs {
		cardIDs32[i] = int32(id)
	}

	queries := dbgen.New(r.Db)
	rows, err := queries.GetCommentCountsByCardIDs(ctx, cardIDs32)
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

func (r *CommentRepository) CreateComment(ctx context.Context, cardID int64, c *model.Comment) (*model.Comment, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.CreateCommentParams{
		Body:     c.Body,
		CardID:   int32(cardID),
		AuthorID: int32(c.AuthorID),
	}

	res, err := queries.CreateComment(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.ID = int64(res.ID)
	c.CreatedAt = res.CreatedAt.Time
	if res.UpdatedAt.Valid {
		c.UpdatedAt = &res.UpdatedAt.Time
	}
	return c, nil
}

func (r *CommentRepository) GetComment(ctx context.Context, id int64) (*model.Comment, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetComment(ctx, int32(id))
	if err != nil {
		return nil, NormalizeError(err)
	}

	comment := &model.Comment{
		ID:        int64(c.ID),
		Body:      c.Body,
		CardID:    int64(c.CardID),
		CreatedAt: c.CreatedAt.Time,
	}
	if c.UpdatedAt.Valid {
		comment.UpdatedAt = &c.UpdatedAt.Time
	}
	comment.AuthorID = int64(c.AuthorID)
	return comment, nil
}

func (r *CommentRepository) UpdateComment(ctx context.Context, c *model.Comment) (*model.Comment, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.UpdateComment(ctx, dbgen.UpdateCommentParams{
		Body: c.Body,
		ID:   int32(c.ID),
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	if res.UpdatedAt.Valid {
		c.UpdatedAt = &res.UpdatedAt.Time
	}
	return c, nil
}

func (r *CommentRepository) DeleteComment(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteComment(ctx, int32(id))
}
