package dto

import (
	"encoding/json"

	"go_kanban_service/internal/model"
)

type CreateSubtaskRequest struct {
	Title    string   `json:"title" validate:"required,max=255"`
	Status   *string  `json:"status,omitempty"`
	Position *float64 `json:"position,omitempty"`
}

type UpdateSubtaskRequest struct {
	Title       *string  `json:"title,omitempty" validate:"omitempty,max=255"`
	Status      *string  `json:"status,omitempty"`
	IsCompleted *bool    `json:"isCompleted,omitempty"`
	Position    *float64 `json:"position,omitempty"`
	UserID      *int64   `json:"user_id,omitempty"`
	HasUserID   bool     `json:"-"`
}

func (r *UpdateSubtaskRequest) UnmarshalJSON(data []byte) error {
	type alias UpdateSubtaskRequest
	payload := struct {
		*alias
		UserIDCamel *int64 `json:"userId"`
	}{
		alias: (*alias)(r),
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if r.UserID == nil && payload.UserIDCamel != nil {
		r.UserID = payload.UserIDCamel
	}
	return nil
}

type SubtaskResponse struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	Status      string  `json:"status"`
	IsCompleted bool    `json:"isCompleted"`
	Position    float64 `json:"position"`
	CardID      int64   `json:"cardId"`
	UserID      *int64  `json:"userId"`
	UserName    *string `json:"userName"`
}

func MapSubtaskResponse(s *model.Subtask) *SubtaskResponse {
	if s == nil {
		return nil
	}
	return &SubtaskResponse{
		ID:          s.ID,
		Title:       s.Title,
		Status:      s.Status,
		IsCompleted: s.Status == "done",
		Position:    s.Position,
		CardID:      s.CardID,
		UserID:      s.UserID,
	}
}

func MapSubtasksResponse(subtasks []model.Subtask) []*SubtaskResponse {
	resp := make([]*SubtaskResponse, 0, len(subtasks))
	for i := range subtasks {
		resp = append(resp, MapSubtaskResponse(&subtasks[i]))
	}
	return resp
}
