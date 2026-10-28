package model

import "time"

// Attachment представляет файл карточки (таблица kanban_attachment).
type Attachment struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	StorageKey  string    `json:"storage_key"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CardID      int64     `json:"card_id"`
	AuthorID    *int64    `json:"author_id,omitempty"`
	AuthorName  *string   `json:"author_name,omitempty"` // не хранится в БД, обогащается из реплики пользователей
	CreatedAt   time.Time `json:"created_at"`
}

// CommentAttachment представляет файл комментария (таблица kanban_card_comment_attachment).
// Те же поля, что у Attachment, только вместо карточки — комментарий, а автора
// нет: файл кладёт только автор комментария.
type CommentAttachment struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	StorageKey  string    `json:"storage_key"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	CommentID   int64     `json:"comment_id"`
	CreatedAt   time.Time `json:"created_at"`
}
