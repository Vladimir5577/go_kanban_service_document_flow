package model

// ProjectUser представляет участника проекта (таблица kanban_project_user).
type ProjectUser struct {
	ID              int64   `json:"id"`
	KanbanProjectID int64   `json:"kanban_project_id"`
	UserID          int64   `json:"user_id"`
	Role            string  `json:"role"`
	FolderID        *int64  `json:"folder_id,omitempty"`
	Position        float64 `json:"position"`
}
