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
	dbComments, err := queries.GetCommentsByCard(ctx, cardID)
	if err != nil {
		return nil, err
	}

	var comments []model.Comment
	for _, c := range dbComments {
		comment := model.Comment{
			ID:        c.ID,
			Body:      c.Body,
			CardID:    c.CardID,
			CreatedAt: c.CreatedAt.Time,
		}
		if c.UpdatedAt.Valid {
			t := c.UpdatedAt.Time
			comment.UpdatedAt = &t
		}
		comment.AuthorID = c.AuthorID
		comments = append(comments, comment)
	}
	return comments, nil
}

func (r *CommentRepository) GetCountsByCardIDs(ctx context.Context, cardIDs []int64) (map[int64]int, error) {
	if len(cardIDs) == 0 {
		return make(map[int64]int), nil
	}

	queries := dbgen.New(r.Db)
	rows, err := queries.GetCommentCountsByCardIDs(ctx, cardIDs)
	if err != nil {
		return nil, err
	}

	result := make(map[int64]int)
	for _, row := range rows {
		result[row.CardID] = int(row.Count)
	}

	return result, nil
}

func (r *CommentRepository) CreateComment(ctx context.Context, cardID int64, c *model.Comment) (*model.Comment, error) {
	queries := dbgen.New(r.Db)

	params := dbgen.CreateCommentParams{
		Body:     c.Body,
		CardID:   cardID,
		AuthorID: c.AuthorID,
	}

	res, err := queries.CreateComment(ctx, params)
	if err != nil {
		return nil, NormalizeError(err)
	}

	c.ID = res.ID
	c.CreatedAt = res.CreatedAt.Time
	if res.UpdatedAt.Valid {
		t := res.UpdatedAt.Time
		c.UpdatedAt = &t
	}
	return c, nil
}

func (r *CommentRepository) GetComment(ctx context.Context, id int64) (*model.Comment, error) {
	queries := dbgen.New(r.Db)
	c, err := queries.GetComment(ctx, id)
	if err != nil {
		return nil, NormalizeError(err)
	}

	comment := &model.Comment{
		ID:        c.ID,
		Body:      c.Body,
		CardID:    c.CardID,
		CreatedAt: c.CreatedAt.Time,
	}
	if c.UpdatedAt.Valid {
		t := c.UpdatedAt.Time
		comment.UpdatedAt = &t
	}
	comment.AuthorID = c.AuthorID
	return comment, nil
}

func (r *CommentRepository) UpdateComment(ctx context.Context, c *model.Comment) (*model.Comment, error) {
	queries := dbgen.New(r.Db)
	res, err := queries.UpdateComment(ctx, dbgen.UpdateCommentParams{
		Body: c.Body,
		ID:   c.ID,
	})
	if err != nil {
		return nil, NormalizeError(err)
	}

	if res.UpdatedAt.Valid {
		t := res.UpdatedAt.Time
		c.UpdatedAt = &t
	}
	return c, nil
}

func (r *CommentRepository) DeleteComment(ctx context.Context, id int64) error {
	queries := dbgen.New(r.Db)
	return queries.DeleteComment(ctx, id)
}
