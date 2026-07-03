package model

import "time"

// Attachment представляет вложение карточки (таблица kanban_attachment).
// Тип вложения задаётся полем Context: 'info' | 'chat' | 'description'.
type Attachment struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	StorageKey  string    `json:"storage_key"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CardID      int64     `json:"card_id"`
	Context     string    `json:"context"`
	AuthorID    *int64    `json:"author_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
