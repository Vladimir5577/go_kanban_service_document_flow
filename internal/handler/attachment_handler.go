package handler

import (
	"net/http"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

// AttachmentHandler — файлы карточки. Файлы комментариев — CommentAttachmentHandler.
type AttachmentHandler struct {
	service  service.AttachmentServiceInterface
	minioSvc service.MinioServiceInterface
	cfg      *config.Config
}

func NewAttachmentHandler(s service.AttachmentServiceInterface, minioSvc service.MinioServiceInterface, cfg *config.Config) *AttachmentHandler {
	return &AttachmentHandler{
		service:  s,
		minioSvc: minioSvc,
		cfg:      cfg,
	}
}

func (h *AttachmentHandler) UploadAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		file := receiveAttachmentFile(w, r, h.minioSvc, h.cfg, cardID)
		if file == nil {
			return
		}

		res, err := h.service.CreateAttachment(r.Context(), cardID, dto.CreateAttachmentRequest{
			Filename:    file.Filename,
			StorageKey:  file.StorageKey,
			ContentType: file.ContentType,
			SizeBytes:   file.SizeBytes,
		})
		if err != nil {
			discardUploadedFile(r, h.minioSvc, h.cfg, file.StorageKey)
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusCreated, dto.MapAttachmentResponse(h.cfg, *res))
	}
}

func (h *AttachmentHandler) DownloadAttachment() http.HandlerFunc {
	return h.serve(false)
}

func (h *AttachmentHandler) PreviewAttachment() http.HandlerFunc {
	return h.serve(true)
}

func (h *AttachmentHandler) serve(inline bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		att, err := h.service.GetAttachment(r.Context(), cardID, id, service.RoleViewer)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		serveAttachmentFile(w, r, h.minioSvc, h.cfg, att.StorageKey, att.Filename, att.ContentType, inline)
	}
}

func (h *AttachmentHandler) DeleteAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		att, err := h.service.GetAttachment(r.Context(), cardID, id, service.RoleEditor)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteAttachment(r.Context(), att); err != nil {
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
