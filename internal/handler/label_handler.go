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

type LabelHandler struct {
	service service.LabelServiceInterface
}

func NewLabelHandler(s service.LabelServiceInterface) *LabelHandler {
	return &LabelHandler{service: s}
}

func (h *LabelHandler) GetLabels() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetLabels(r.Context(), boardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapLabelsResponse(res))
	}
}

func (h *LabelHandler) CreateLabel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.CreateLabelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.CreateLabel(r.Context(), boardID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, dto.MapLabelResponse(res))
	}
}

func (h *LabelHandler) DeleteLabel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		labelID, err := helper.IDParam(r, "labelId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteLabel(r.Context(), labelID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}

func (h *LabelHandler) ToggleLabel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		labelID, err := helper.IDParam(r, "labelId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.ToggleLabel(r.Context(), cardID, labelID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, map[string]string{"status": "toggled"})
	}
}
