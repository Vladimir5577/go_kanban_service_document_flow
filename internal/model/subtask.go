package model

// Subtask представляет подзадачу-пункт чеклиста (таблица kanban_card_subtask).
type Subtask struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Status   string  `json:"status"`
	Position float64 `json:"position"`
	CardID   int64   `json:"card_id"`
	UserID   *int64  `json:"user_id,omitempty"`
}

// ChecklistCount содержит агрегатные данные по чеклисту карточки.
type ChecklistCount struct {
	Total int
	Done  int
}
