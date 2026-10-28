package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/google/uuid"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

// uploadedFile — файл из multipart-формы, уже лежащий в MinIO.
type uploadedFile struct {
	Filename    string
	StorageKey  string
	ContentType string
	SizeBytes   int64
}

// receiveAttachmentFile читает поле `file` и кладёт объект в MinIO под cardID.
// Ошибку сам пишет в ответ и возвращает nil.
func receiveAttachmentFile(w http.ResponseWriter, r *http.Request, minioSvc service.MinioServiceInterface, cfg *config.Config, cardID int64) *uploadedFile {
	if err := r.ParseMultipartForm(50 << 20); err != nil { // 50 MB
		helper.WriteError(w, apperr.New(apperr.CodeFileNotProvided, "file not provided"))
		return nil
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		helper.WriteError(w, apperr.New(apperr.CodeFileNotProvided, "file not provided"))
		return nil
	}
	defer file.Close()

	// Ключ объекта НЕ должен содержать исходное имя файла: MinIO хранит объекты
	// как файлы, а сегмент пути в ФС ограничен 255 байтами. Имя >218 символов
	// раньше падало с "Object name contains unsupported characters" → 500.
	// Уникальность даёт uuid, расширение сохраняем для content-type/preview.
	// Читаемое имя живёт в БД (Filename) и отдаётся в Content-Disposition.
	ext := filepath.Ext(header.Filename)
	if len(ext) > 16 { // защита от патологического "расширения"
		ext = ""
	}
	objectName := fmt.Sprintf("%d/%s%s", cardID, uuid.New().String(), ext)

	contentType := header.Header.Get("Content-Type")
	if err := minioSvc.UploadFile(r.Context(), cfg.MinioBucket, objectName, file, header.Size, contentType); err != nil {
		helper.WriteError(w, err)
		return nil
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return &uploadedFile{
		Filename:    header.Filename,
		StorageKey:  objectName,
		ContentType: contentType,
		SizeBytes:   header.Size,
	}
}

// discardUploadedFile убирает объект из MinIO, если запись в БД не удалась.
func discardUploadedFile(r *http.Request, minioSvc service.MinioServiceInterface, cfg *config.Config, storageKey string) {
	if err := minioSvc.DeleteObject(r.Context(), cfg.MinioBucket, storageKey); err != nil {
		slog.WarnContext(r.Context(), "failed to cleanup uploaded attachment after database error", "storage_key", storageKey, "error", err)
	}
}

// serveAttachmentFile отдаёт объект из MinIO: inline для превью, иначе на скачивание.
func serveAttachmentFile(w http.ResponseWriter, r *http.Request, minioSvc service.MinioServiceInterface, cfg *config.Config, storageKey, filename, contentType string, inline bool) {
	obj, err := minioSvc.GetObject(r.Context(), cfg.MinioBucket, storageKey)
	if err != nil {
		helper.WriteError(w, apperr.New(apperr.CodeFileNotFoundOnDisk, "file not found on disk"))
		return
	}
	defer obj.Close()

	if inline {
		w.Header().Set("Content-Disposition", "inline")
	} else {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	}
	w.Header().Set("Content-Type", contentType)
	io.Copy(w, obj)
}
