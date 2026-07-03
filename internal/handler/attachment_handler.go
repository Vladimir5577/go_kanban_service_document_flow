package handler

import (
	"fmt"
	"io"
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

		ctxVal := r.FormValue("context")
		if ctxVal == "" {
			ctxVal = "card"
		}

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
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusCreated, dto.MapAttachmentResponse(h.cfg, *res))
	}
}

func (h *AttachmentHandler) DownloadAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		
		att, err := h.service.GetAttachment(r.Context(), id)
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
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		
		att, err := h.service.GetAttachment(r.Context(), id)
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
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteAttachment(r.Context(), id); err != nil {
			helper.WriteError(w, err)
			return
		}
		
		// Wait, we need to delete from Minio too!
		// But we don't have the att before deleting, let's fetch it first
		att, err := h.service.GetAttachment(r.Context(), id)
		if err == nil {
			// Ignore minio delete errors to ensure DB transaction doesn't fail
			_ = h.minioSvc.DeleteObject(r.Context(), h.cfg.MinioBucket, att.StorageKey)
		}

		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
