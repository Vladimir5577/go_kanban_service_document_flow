package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

type AttachmentHandler struct {
	service service.AttachmentServiceInterface
}

func NewAttachmentHandler(s service.AttachmentServiceInterface) *AttachmentHandler {
	return &AttachmentHandler{service: s}
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

		uploadDir := "uploads/attachments"
		if err := os.MkdirAll(uploadDir, os.ModePerm); err != nil {
			helper.WriteError(w, err)
			return
		}

		storageKey := filepath.Join(uploadDir, fmt.Sprintf("%d_%s", time.Now().UnixNano(), header.Filename))
		outFile, err := os.Create(storageKey)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		defer outFile.Close()

		if _, err := io.Copy(outFile, file); err != nil {
			helper.WriteError(w, err)
			return
		}

		contentType := header.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		req := dto.CreateAttachmentRequest{
			Filename:    header.Filename,
			StorageKey:  storageKey,
			ContentType: contentType,
			SizeBytes:   header.Size,
			Context:     ctxVal,
		}
		res, err := h.service.CreateAttachment(r.Context(), cardID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, dto.MapAttachmentResponse(res))
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

		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", att.Filename))
		w.Header().Set("Content-Type", att.ContentType)
		http.ServeFile(w, r, att.StorageKey)
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

		w.Header().Set("Content-Disposition", "inline")
		w.Header().Set("Content-Type", att.ContentType)
		http.ServeFile(w, r, att.StorageKey)
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
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
