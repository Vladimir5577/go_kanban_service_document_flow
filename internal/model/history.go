package model

import "time"

type HistoryWrite struct {
	ProjectID   int64
	Action      string
	EntityType  string
	EntityID    int64
	CardID      int64
	EntityTitle string
	EntityLink  string
	Before      string
	After       string
}

type HistoryPayload struct {
	Before *string `json:"before"`
	After  *string `json:"after"`
}

type HistoryListItem struct {
	ID          int64
	Action      string
	EntityTitle string
	EntityLink  string
	Before      string
	After       string
	UserID      *int64
	UserName    string
	CreatedAt   string
}

type HistoryListPage struct {
	Items      []HistoryListItem
	HasMore    bool
	NextCursor int64
}

type HistoryUndoEntry struct {
	ID          int64
	Action      string
	EntityType  string
	EntityID    int64
	CardID      int64
	EntityTitle string
	ProjectID   int64
	Before      string
	After       string
	CreatedAt   time.Time
}
