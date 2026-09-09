package model

import "encoding/json"

type HistoryWrite struct {
	ProjectID    int64
	Action       string
	EntityTitle  string
	EntityLink   string
	Before       string
	After        string
	EntityKeys   []string
	Undo         []UndoStep
}

type UndoStep struct {
	Op        string          `json:"op"`
	CardID    int64           `json:"cardId,omitempty"`
	ColumnID  int64           `json:"columnId,omitempty"`
	BoardID   int64           `json:"boardId,omitempty"`
	ProjectID int64           `json:"projectId,omitempty"`
	LabelID   int64           `json:"labelId,omitempty"`
	CommentID int64           `json:"commentId,omitempty"`
	SubtaskID int64           `json:"subtaskId,omitempty"`
	AttachID  int64           `json:"attachmentId,omitempty"`
	UserID    int64           `json:"userId,omitempty"`
	UserIDs   []int64         `json:"userIds,omitempty"`
	Position  *float64        `json:"position,omitempty"`
	On        *bool           `json:"on,omitempty"`
	Fields    json.RawMessage `json:"fields,omitempty"`
	Snapshot  json.RawMessage `json:"snapshot,omitempty"`
}

type HistoryPayload struct {
	Steps  []UndoStep `json:"steps"`
	Before string     `json:"before,omitempty"`
	After  string     `json:"after,omitempty"`
}

type HistoryListItem struct {
	ID          int64
	Action      string
	EntityTitle string
	EntityLink  string
	Before      string
	After       string
	UserID    *int64
	UserName  string
	CreatedAt string
}

type HistoryListPage struct {
	Items      []HistoryListItem
	HasMore    bool
	NextCursor int64
}
