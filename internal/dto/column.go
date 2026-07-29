package dto

import (
	"encoding/json"

	"go_kanban_service/internal/model"
)

type CreateColumnRequest struct {
	Title       string   `json:"title" validate:"required,min=1,max=70"`
	HeaderColor *string  `json:"headerColor,omitempty"`
	Position    *float64 `json:"position,omitempty"`
}

func (r *CreateColumnRequest) UnmarshalJSON(data []byte) error {
	var payload struct {
		Title             string   `json:"title"`
		HeaderColor       *string  `json:"headerColor"`
		HeaderColorLegacy *string  `json:"header_color"`
		Position          *float64 `json:"position"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	r.Title = payload.Title
	r.HeaderColor = payload.HeaderColor
	if r.HeaderColor == nil {
		r.HeaderColor = payload.HeaderColorLegacy
	}
	r.Position = payload.Position
	return nil
}

type UpdateColumnRequest struct {
	Title       *string  `json:"title,omitempty" validate:"omitempty,max=70"`
	HeaderColor *string  `json:"headerColor,omitempty"`
	Position    *float64 `json:"position,omitempty"`
}

func (r *UpdateColumnRequest) UnmarshalJSON(data []byte) error {
	var payload struct {
		Title             *string  `json:"title"`
		HeaderColor       *string  `json:"headerColor"`
		HeaderColorLegacy *string  `json:"header_color"`
		Position          *float64 `json:"position"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	r.Title = payload.Title
	r.HeaderColor = payload.HeaderColor
	if r.HeaderColor == nil {
		r.HeaderColor = payload.HeaderColorLegacy
	}
	r.Position = payload.Position
	return nil
}

type ColumnResponse struct {
	ID          int64           `json:"id"`
	Title       string          `json:"title"`
	HeaderColor string          `json:"headerColor"`
	Position    float64         `json:"position"`
	BoardID     int64           `json:"boardId"`
	Cards       []*CardResponse `json:"cards"`
}

func MapColumnResponse(c *model.Column) *ColumnResponse {
	if c == nil {
		return nil
	}
	return &ColumnResponse{
		ID:          c.ID,
		Title:       c.Title,
		HeaderColor: c.HeaderColor,
		Position:    c.Position,
		BoardID:     c.BoardID,
		Cards:       make([]*CardResponse, 0),
	}
}

func MapColumnsResponse(columns []model.Column) []*ColumnResponse {
	resp := make([]*ColumnResponse, 0, len(columns))
	for i := range columns {
		resp = append(resp, MapColumnResponse(&columns[i]))
	}
	return resp
}
