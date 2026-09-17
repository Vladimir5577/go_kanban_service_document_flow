package dto

import "time"

type TaskRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type TaskProjectRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type TaskListItem struct {
	ID          int64           `json:"id"`
	Title       string          `json:"title"`
	Priority    *string         `json:"priority"`
	DueDate     *time.Time      `json:"dueDate"`
	BorderColor *string         `json:"borderColor"`
	ParentID    *int64          `json:"parentId"`
	ParentTitle *string         `json:"parentTitle"`
	Column      TaskRef         `json:"column"`
	Board       TaskRef         `json:"board"`
	Project     TaskProjectRef  `json:"project"`
	CreatedAt   time.Time       `json:"createdAt"`
	CompletedAt *time.Time      `json:"completedAt"`
	ArchivedAt  *time.Time      `json:"archivedAt"`
	IsArchived  bool            `json:"isArchived"`
	Assignee    *UserResponse   `json:"assignee"`
}

type TaskListResponse struct {
	Items []*TaskListItem `json:"items"`
	Total int64           `json:"total"`
}

type TaskCollaborantsResponse struct {
	Items []*UserResponse `json:"items"`
	Total int64           `json:"total"`
}
