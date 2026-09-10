package dto

import (
	"strings"

	"go_kanban_service/internal/model"
)

type HistoryUserResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type HistoryItemResponse struct {
	ID        int64                `json:"id"`
	Action      string               `json:"action"`
	EntityTitle string               `json:"entityTitle"`
	EntityLink  string               `json:"entityLink"`
	Before      string               `json:"before,omitempty"`
	After     string               `json:"after,omitempty"`
	CreatedAt string               `json:"createdAt"`
	User      *HistoryUserResponse `json:"user"`
}

type HistoryListResponse struct {
	Items      []HistoryItemResponse `json:"items"`
	HasMore    bool                  `json:"hasMore"`
	NextCursor int64                 `json:"nextCursor"`
}

type HistoryUndoResponse struct {
	Action      string `json:"action"`
	EntityTitle string `json:"entityTitle"`
}

func MapHistoryList(items []model.HistoryListItem, hasMore bool, nextCursor int64) HistoryListResponse {
	resp := HistoryListResponse{
		Items:      make([]HistoryItemResponse, 0, len(items)),
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}
	for _, item := range items {
		row := HistoryItemResponse{
			ID:        item.ID,
			Action:      item.Action,
			EntityTitle: item.EntityTitle,
			EntityLink:  item.EntityLink,
			Before:    clipHistory(item.Before),
			After:     clipHistory(item.After),
			CreatedAt: item.CreatedAt,
		}
		if item.UserID != nil {
			row.User = &HistoryUserResponse{ID: *item.UserID, Name: item.UserName}
		}
		resp.Items = append(resp.Items, row)
	}
	return resp
}

func clipHistory(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}
