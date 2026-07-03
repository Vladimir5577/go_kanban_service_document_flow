package dto

import (
	"fmt"
	"time"

	"go_kanban_service/internal/model"
)

type CreateAttachmentRequest struct {
	Filename    string `json:"filename" validate:"required"`
	StorageKey  string `json:"storage_key" validate:"required"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
	Context     string `json:"context"`
}

type AttachmentResponse struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	StorageKey  string    `json:"storageKey"`
	ContentType string    `json:"contentType"`
	SizeBytes   int64     `json:"sizeBytes"`
	CardID      int64     `json:"cardId"`
	Context     string    `json:"context"`
	AuthorID    *int64    `json:"authorId,omitempty"`
	AuthorName  *string   `json:"authorName,omitempty"`
	PreviewUrl  *string   `json:"previewUrl,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

func MapAttachmentResponse(a *model.Attachment) *AttachmentResponse {
	if a == nil {
		return nil
	}
	previewUrl := fmt.Sprintf("/spa/api/cards/%d/attachments/%d/preview", a.CardID, a.ID)
	return &AttachmentResponse{
		ID:          a.ID,
		Filename:    a.Filename,
		StorageKey:  a.StorageKey,
		ContentType: a.ContentType,
		SizeBytes:   a.SizeBytes,
		CardID:      a.CardID,
		Context:     a.Context,
		AuthorID:    a.AuthorID,
		PreviewUrl:  &previewUrl,
		CreatedAt:   a.CreatedAt,
	}
}

func MapAttachmentsResponse(attachments []model.Attachment) []*AttachmentResponse {
	resp := make([]*AttachmentResponse, 0, len(attachments))
	for i := range attachments {
		resp = append(resp, MapAttachmentResponse(&attachments[i]))
	}
	return resp
}
