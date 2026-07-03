package dto

import (
	"time"

	"go_kanban_service/internal/model"
)

type CreateBoardRequest struct {
	Title    string   `json:"title" validate:"required,min=2,max=255"`
	Position *float64 `json:"position,omitempty"`
}

type UpdateBoardRequest struct {
	Title    *string  `json:"title,omitempty" validate:"omitempty,min=2,max=255"`
	Position *float64 `json:"position,omitempty"`
}

type BoardResponse struct {
	ID              int64             `json:"id"`
	Title           string            `json:"title"`
	Position        float64           `json:"position"`
	KanbanProjectID int64             `json:"kanbanProjectId"`
	CreatedByID     int64             `json:"createdById"`
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
	Columns         []*ColumnResponse `json:"columns"`
}

func MapBoardResponse(b *model.Board) *BoardResponse {
	if b == nil {
		return nil
	}
	return &BoardResponse{
		ID:              b.ID,
		Title:           b.Title,
		Position:        b.Position,
		KanbanProjectID: b.KanbanProjectID,
		CreatedByID:     b.CreatedByID,
		CreatedAt:       b.CreatedAt,
		UpdatedAt:       b.UpdatedAt,
		Columns:         make([]*ColumnResponse, 0),
	}
}

func MapBoardsResponse(boards []model.Board) []*BoardResponse {
	resp := make([]*BoardResponse, 0, len(boards))
	for i := range boards {
		resp = append(resp, MapBoardResponse(&boards[i]))
	}
	return resp
}
