package dto

import "go_kanban_service/internal/model"

type CreateLabelRequest struct {
	Name  string `json:"name" validate:"required,max=50"`
	Color string `json:"color" validate:"required,max=20"`
}

type UpdateLabelRequest struct {
	Name  *string `json:"name,omitempty" validate:"omitempty,max=50"`
	Color *string `json:"color,omitempty" validate:"omitempty,max=20"`
}

type LabelResponse struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	BoardID int64  `json:"boardId"`
}

func MapLabelResponse(l *model.Label) *LabelResponse {
	if l == nil {
		return nil
	}
	return &LabelResponse{
		ID:      l.ID,
		Name:    l.Name,
		Color:   l.Color,
		BoardID: l.BoardID,
	}
}

func MapLabelsResponse(labels []model.Label) []*LabelResponse {
	resp := make([]*LabelResponse, 0, len(labels))
	for i := range labels {
		resp = append(resp, MapLabelResponse(&labels[i]))
	}
	return resp
}
