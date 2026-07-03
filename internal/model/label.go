package model

// Label представляет метку доски (таблица kanban_label).
type Label struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Color   string `json:"color"`
	BoardID int64  `json:"board_id"`
}
