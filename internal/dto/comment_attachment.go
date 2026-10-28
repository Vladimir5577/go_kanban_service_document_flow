package dto

import (
	"fmt"
	"time"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/model"
)

type CreateCommentAttachmentRequest struct {
	Filename    string `json:"filename" validate:"required"`
	StorageKey  string `json:"storage_key" validate:"required"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	// Файл летит вместе с отправкой комментария: в истории его накрывает
	// запись comment.created, своей он не получает.
	SkipHistory bool `json:"skip_history"`
}

// CommentAttachmentResponse — те же поля, что у AttachmentResponse, плюс commentId:
// фронт рисует оба вида файлов одним компонентом.
type CommentAttachmentResponse struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	StorageKey  string    `json:"storageKey"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	CardID      int64     `json:"cardId"`
	CommentID   int64     `json:"commentId"`
	PreviewUrl  *string   `json:"previewUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// MapCommentAttachmentResponse. cardID нужен для пути превью — в строке файла
// комментария карточки нет.
func MapCommentAttachmentResponse(cfg *config.Config, cardID int64, a model.CommentAttachment) *CommentAttachmentResponse {
	previewUrl := attachmentPreviewURL(cfg, a.ContentType, a.StorageKey,
		fmt.Sprintf("/spa/api/kanban/cards/%d/comments/%d/attachments/%d/preview", cardID, a.CommentID, a.ID))

	return &CommentAttachmentResponse{
		ID:          a.ID,
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		CardID:      cardID,
		CommentID:   a.CommentID,
		PreviewUrl:  &previewUrl,
		CreatedAt:   a.CreatedAt,
	}
}

func MapCommentAttachmentsResponse(cfg *config.Config, cardID int64, attachments []model.CommentAttachment) []*CommentAttachmentResponse {
	res := make([]*CommentAttachmentResponse, 0, len(attachments))
	for _, a := range attachments {
		res = append(res, MapCommentAttachmentResponse(cfg, cardID, a))
	}
	return res
}
