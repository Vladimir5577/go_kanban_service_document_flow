package handler

import (
	"net/http"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

// CommentAttachmentHandler — файлы комментария:
// /spa/api/kanban/cards/{cardId}/comments/{commentId}/attachments.
type CommentAttachmentHandler struct {
	service  service.CommentAttachmentServiceInterface
	minioSvc service.MinioServiceInterface
	cfg      *config.Config
}

func NewCommentAttachmentHandler(s service.CommentAttachmentServiceInterface, minioSvc service.MinioServiceInterface, cfg *config.Config) *CommentAttachmentHandler {
	return &CommentAttachmentHandler{
		service:  s,
		minioSvc: minioSvc,
		cfg:      cfg,
	}
}

func commentAttachmentIDs(r *http.Request) (cardID, commentID int64, err error) {
	if cardID, err = helper.IDParam(r, "cardId"); err != nil {
		return 0, 0, err
	}
	if commentID, err = helper.IDParam(r, "commentId"); err != nil {
		return 0, 0, err
	}
	return cardID, commentID, nil
}

// UploadAttachment. Поле `initial=1` — файл летит вместе с отправкой
// комментария и не получает своей записи в истории.
func (h *CommentAttachmentHandler) UploadAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, commentID, err := commentAttachmentIDs(r)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		file := receiveAttachmentFile(w, r, h.minioSvc, h.cfg, cardID)
		if file == nil {
			return
		}

		res, err := h.service.CreateAttachment(r.Context(), cardID, commentID, dto.CreateCommentAttachmentRequest{
			Filename:    file.Filename,
			StorageKey:  file.StorageKey,
			ContentType: file.ContentType,
			SizeBytes:   file.SizeBytes,
			SkipHistory: r.FormValue("initial") == "1",
		})
		if err != nil {
			discardUploadedFile(r, h.minioSvc, h.cfg, file.StorageKey)
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusCreated, dto.MapCommentAttachmentResponse(h.cfg, cardID, *res))
	}
}

func (h *CommentAttachmentHandler) DownloadAttachment() http.HandlerFunc {
	return h.serve(false)
}

func (h *CommentAttachmentHandler) PreviewAttachment() http.HandlerFunc {
	return h.serve(true)
}

func (h *CommentAttachmentHandler) serve(inline bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, commentID, err := commentAttachmentIDs(r)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		att, err := h.service.GetAttachment(r.Context(), cardID, commentID, id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		serveAttachmentFile(w, r, h.minioSvc, h.cfg, att.StorageKey, att.Filename, att.ContentType, inline)
	}
}

func (h *CommentAttachmentHandler) DeleteAttachment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, commentID, err := commentAttachmentIDs(r)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteAttachment(r.Context(), cardID, commentID, id); err != nil {
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
