package handler

import (
	"encoding/json"
	"net/http"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
	"go_kanban_service/internal/validator"
)

type CommentHandler struct {
	service service.CommentServiceInterface
}

func NewCommentHandler(s service.CommentServiceInterface) *CommentHandler {
	return &CommentHandler{service: s}
}

func (h *CommentHandler) GetComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetComments(r.Context(), cardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapCommentsResponse(res))
	}
}

func (h *CommentHandler) CreateComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.CreateCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, map[validationCodeKey]apperr.ErrorCode{
				{Field: "Body", Tag: "required"}: apperr.CodeCommentBodyRequired,
			}))
			return
		}

		res, err := h.service.CreateComment(r.Context(), cardID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, dto.MapCommentResponse(res))
	}
}

func (h *CommentHandler) UpdateComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		commentID, err := helper.IDParam(r, "commentId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, map[validationCodeKey]apperr.ErrorCode{
				{Field: "Body", Tag: "required"}: apperr.CodeCommentBodyRequired,
			}))
			return
		}

		res, err := h.service.UpdateComment(r.Context(), cardID, commentID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapCommentResponse(res))
	}
}

func (h *CommentHandler) DeleteComment() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		commentID, err := helper.IDParam(r, "commentId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteComment(r.Context(), cardID, commentID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
