package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

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

		err = r.ParseMultipartForm(50 << 20) // 50 MB
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		defer file.Close()

		ctxVal := normalizeAttachmentContext(r.FormValue("context"))
		objectName := fmt.Sprintf("cards/%d/%s-%s", cardID, uuid.New().String(), header.Filename)

		err = h.minioSvc.UploadFile(r.Context(), h.cfg.MinioBucket, objectName, file, header.Size, header.Header.Get("Content-Type"))
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		req := dto.CreateAttachmentRequest{
			Filename:    header.Filename,
			StorageKey:  objectName,
			ContentType: contentType,
			SizeBytes:   header.Size,
			Context:     ctxVal,
		}
		res, err := h.service.CreateAttachment(r.Context(), cardID, req)
		if err != nil {
			if cleanupErr := h.minioSvc.DeleteObject(r.Context(), h.cfg.MinioBucket, objectName); cleanupErr != nil {
				slog.WarnContext(r.Context(), "failed to cleanup uploaded attachment after database error", "storage_key", objectName, "error", cleanupErr)
			}
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusCreated, dto.MapAttachmentResponse(h.cfg, *res))
	}
}

func (h *AttachmentHandler) DownloadAttachment() http.HandlerFunc {
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

		obj, err := h.minioSvc.GetObject(r.Context(), h.cfg.MinioBucket, att.StorageKey)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		defer obj.Close()

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.Filename))
		w.Header().Set("Content-Type", att.ContentType)
		io.Copy(w, obj)
	}
}

func (h *AttachmentHandler) PreviewAttachment() http.HandlerFunc {
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

		obj, err := h.minioSvc.GetObject(r.Context(), h.cfg.MinioBucket, att.StorageKey)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		defer obj.Close()

		w.Header().Set("Content-Disposition", "inline")
		w.Header().Set("Content-Type", att.ContentType)
		io.Copy(w, obj)
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

		_ = h.minioSvc.DeleteObject(r.Context(), h.cfg.MinioBucket, att.StorageKey)
		if err := h.service.DeleteAttachment(r.Context(), att); err != nil {
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}

func normalizeAttachmentContext(value string) string {
	switch value {
	case "chat", "info", "description":
		return value
	default:
		return "info"
	}
}
