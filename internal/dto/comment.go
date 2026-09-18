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

type MarkCommentsReadRequest struct {
	LastCommentID *int64 `json:"lastCommentId" validate:"required,gt=0"`
}

// CommentReaderResponse — строка модалки «кто видел».
type CommentReaderResponse struct {
	User           *UserResponse `json:"user"`
	ReadAt         time.Time     `json:"readAt"`
	ReadBeforeEdit bool          `json:"readBeforeEdit"`
}

type CommentReadersResponse struct {
	Readers []*CommentReaderResponse `json:"readers"`
	Pending []*UserResponse          `json:"pending"`
}

func MapCommentReadersResponse(r *model.CommentReaders) *CommentReadersResponse {
	if r == nil {
		return nil
	}
	resp := &CommentReadersResponse{
		Readers: make([]*CommentReaderResponse, 0, len(r.Readers)),
		Pending: MapUsersResponse(r.Pending),
	}
	for i := range r.Readers {
		resp.Readers = append(resp.Readers, &CommentReaderResponse{
			User:           MapUserResponse(&r.Readers[i].User),
			ReadAt:         r.Readers[i].ReadAt,
			ReadBeforeEdit: r.Readers[i].ReadBeforeEdit,
		})
	}
	return resp
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
