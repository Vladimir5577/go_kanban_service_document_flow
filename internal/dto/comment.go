package dto

import (
	"time"

	"go_kanban_service/internal/model"
)

type CreateCommentRequest struct {
	Body string `json:"body" validate:"required"`
}

type UpdateCommentRequest struct {
	Body *string `json:"body" validate:"required"`
}

type CommentResponse struct {
	ID         int64      `json:"id"`
	Body       string     `json:"body"`
	CardID     int64      `json:"cardId"`
	AuthorID   int64      `json:"authorId"`
	AuthorName string     `json:"authorName"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

func MapCommentResponse(c *model.Comment) *CommentResponse {
	if c == nil {
		return nil
	}
	return &CommentResponse{
		ID:         c.ID,
		Body:       c.Body,
		CardID:     c.CardID,
		AuthorID:   c.AuthorID,
		AuthorName: c.AuthorName,
		CreatedAt:  c.CreatedAt,
		UpdatedAt:  c.UpdatedAt,
	}
}

func MapCommentsResponse(comments []model.Comment) []*CommentResponse {
	resp := make([]*CommentResponse, 0, len(comments))
	for i := range comments {
		resp = append(resp, MapCommentResponse(&comments[i]))
	}
	return resp
}
