package handler

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

const maxAttachmentBodyBytes = 50 << 20

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

		if err := h.service.AuthorizeUpload(r.Context(), cardID); err != nil {
			helper.WriteError(w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxAttachmentBodyBytes)
		err = r.ParseMultipartForm(1 << 20)
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			helper.WriteError(w, apperr.New(apperr.CodeFileNotProvided, "file not provided"))
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			helper.WriteError(w, apperr.New(apperr.CodeFileNotProvided, "file not provided"))
			return
		}
		defer file.Close()
		if header.Size > maxAttachmentBodyBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		prefix := make([]byte, 512)
		n, err := io.ReadFull(file, prefix)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			helper.WriteError(w, err)
			return
		}
		contentType := http.DetectContentType(prefix[:n])
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			helper.WriteError(w, err)
			return
		}

		ctxVal := normalizeAttachmentContext(r.FormValue("context"))
		// Ключ объекта НЕ должен содержать исходное имя файла: MinIO хранит объекты
		// как файлы, а сегмент пути в ФС ограничен 255 байтами. Имя >218 символов
		// раньше падало с "Object name contains unsupported characters" → 500.
		// Уникальность даёт uuid, расширение сохраняем для content-type/preview.
		// Читаемое имя живёт в БД (att.Filename) и отдаётся в Content-Disposition.
		ext := filepath.Ext(header.Filename)
		if len(ext) > 16 { // защита от патологического "расширения"
			ext = ""
		}
		objectName := fmt.Sprintf("%d/%s%s", cardID, uuid.New().String(), ext)

		err = h.minioSvc.UploadFile(r.Context(), h.cfg.MinioBucket, objectName, file, header.Size, contentType)
		if err != nil {
			helper.WriteError(w, err)
			return
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
			writeAttachmentStorageError(w, err)
			return
		}
		defer obj.Close()
		reader := bufio.NewReader(obj)
		if _, err := reader.Peek(1); err != nil && err != io.EOF {
			writeAttachmentStorageError(w, err)
			return
		}

		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": att.Filename}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Type", att.ContentType)
		if _, err := io.Copy(w, reader); err != nil {
			slog.WarnContext(r.Context(), "failed to stream attachment download", "error", err)
			// Обычный return закончил бы обрезанный ответ как успешный.
			panic(http.ErrAbortHandler)
		}
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
			writeAttachmentStorageError(w, err)
			return
		}
		defer obj.Close()

		// Ошибка здесь только из io.Copy, когда заголовки уже ушли.
		if err := writeAttachmentPreview(w, obj, att.Filename); err != nil {
			slog.WarnContext(r.Context(), "failed to stream attachment preview", "error", err)
			panic(http.ErrAbortHandler)
		}
	}
}

func writeAttachmentPreview(w http.ResponseWriter, src io.Reader, filename string) error {
	prefix := make([]byte, 512)
	n, err := io.ReadFull(src, prefix)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		writeAttachmentStorageError(w, err)
		return nil
	}
	contentType := http.DetectContentType(prefix[:n])
	disposition := "attachment"
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": filename}))
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, err = io.Copy(w, io.MultiReader(bytes.NewReader(prefix[:n]), src))
	return err
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

func normalizeAttachmentContext(value string) string {
	switch value {
	case "chat", "info", "description":
		return value
	default:
		return "info"
	}
}
