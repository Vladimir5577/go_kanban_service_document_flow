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

type SubtaskHandler struct {
	service service.SubtaskServiceInterface
}

func NewSubtaskHandler(s service.SubtaskServiceInterface) *SubtaskHandler {
	return &SubtaskHandler{service: s}
}

func (h *SubtaskHandler) GetSubtasks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetSubtasks(r.Context(), cardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapSubtasksResponse(res))
	}
}

func (h *SubtaskHandler) CreateSubtask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.CreateSubtaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.CreateSubtask(r.Context(), cardID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, dto.MapSubtaskResponse(res))
	}
}

func (h *SubtaskHandler) UpdateSubtask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subtaskID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateSubtaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.UpdateSubtask(r.Context(), subtaskID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapSubtaskResponse(res))
	}
}

func (h *SubtaskHandler) DeleteSubtask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subtaskID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteSubtask(r.Context(), subtaskID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
