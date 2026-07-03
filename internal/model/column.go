package model

// Column представляет колонку доски (таблица kanban_column).
type Column struct {
	ID          int64   `json:"id"`
	Title       string  `json:"title"`
	HeaderColor string  `json:"header_color"`
	Position    float64 `json:"position"`
	BoardID     int64   `json:"board_id"`
}
