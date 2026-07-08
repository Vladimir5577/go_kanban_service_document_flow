package dto

import (
	"fmt"
	"strings"
	"time"

	"go_kanban_service/internal/config"
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

func MapAttachmentResponse(cfg *config.Config, a model.Attachment) *AttachmentResponse {
	var previewUrl string
	if strings.HasPrefix(a.ContentType, "image/") && cfg.ImgproxyBaseUrl != "" {
		// e.g. http://localhost:8082/unsafe/rs:fit:400:400/plain/s3://kanban/cards/...
		previewUrl = fmt.Sprintf("%s/unsafe/rs:fit:400:400/plain/s3://%s/%s",
			strings.TrimRight(cfg.ImgproxyBaseUrl, "/"),
			cfg.MinioBucket,
			a.StorageKey)
	} else {
		previewUrl = fmt.Sprintf("/spa/api/kanban/cards/%d/attachments/%d/preview", a.CardID, a.ID)
	}

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

func MapAttachmentsResponse(cfg *config.Config, attachments []model.Attachment) []*AttachmentResponse {
	if attachments == nil {
		return []*AttachmentResponse{}
	}
	res := make([]*AttachmentResponse, 0, len(attachments))
	for _, a := range attachments {
		res = append(res, MapAttachmentResponse(cfg, a))
	}
	return res
}
