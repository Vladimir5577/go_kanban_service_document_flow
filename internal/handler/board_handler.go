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

type BoardHandler struct {
	service service.BoardServiceInterface
}

func NewBoardHandler(s service.BoardServiceInterface) *BoardHandler {
	return &BoardHandler{service: s}
}

func (h *BoardHandler) CreateBoard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.CreateBoardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.CreateBoard(r.Context(), projectID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, dto.MapBoardResponse(res))
	}
}

func (h *BoardHandler) GetBoard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetBoard(r.Context(), boardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *BoardHandler) UpdateBoard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateBoardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: malformed JSON body", apperr.ErrValidation))
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, fmt.Errorf("%w: validation error: %v", apperr.ErrValidation, err))
			return
		}

		res, err := h.service.UpdateBoard(r.Context(), boardID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapBoardResponse(res))
	}
}

func (h *BoardHandler) DeleteBoard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteBoard(r.Context(), boardID); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}

func (h *BoardHandler) GetBoardArchive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetBoardArchive(r.Context(), boardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapCardsResponse(res))
	}
}
