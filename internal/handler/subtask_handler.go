package handler

import (
	"encoding/json"
	"fmt"
	"io"
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
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		subtaskID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			helper.WriteError(w, fmt.Errorf("%w: cannot read body", apperr.ErrValidation))
			return
		}

		var req dto.UpdateSubtaskRequest
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}

		var raw map[string]interface{}
		_ = json.Unmarshal(bodyBytes, &raw)
		if _, ok := raw["user_id"]; ok {
			req.HasUserID = true
		}
		if _, ok := raw["userId"]; ok {
			req.HasUserID = true
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.UpdateSubtask(r.Context(), cardID, subtaskID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapSubtaskResponse(res))
	}
}

func (h *SubtaskHandler) DeleteSubtask() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		subtaskID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteSubtask(r.Context(), cardID, subtaskID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}
