package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
	"go_kanban_service/internal/validator"
)

type ColumnHandler struct {
	service service.ColumnServiceInterface
}

func NewColumnHandler(s service.ColumnServiceInterface) *ColumnHandler {
	return &ColumnHandler{service: s}
}

func (h *ColumnHandler) CreateColumn() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.CreateColumnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.CreateColumn(r.Context(), boardID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, dto.MapColumnResponse(res))
	}
}

func (h *ColumnHandler) UpdateColumn() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		columnID, err := helper.IDParam(r, "columnId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateColumnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.UpdateColumn(r.Context(), columnID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapColumnResponse(res))
	}
}

func (h *ColumnHandler) DeleteColumn() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		columnID, err := helper.IDParam(r, "columnId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteColumn(r.Context(), columnID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
