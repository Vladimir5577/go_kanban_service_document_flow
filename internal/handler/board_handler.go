package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/model"
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
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetBoard(r.Context(), projectID, boardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *BoardHandler) UpdateBoard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

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

		res, err := h.service.UpdateBoard(r.Context(), projectID, boardID, req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapBoardResponse(res))
	}
}

func (h *BoardHandler) DeleteBoard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.DeleteBoard(r.Context(), projectID, boardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *BoardHandler) GetBoardArchive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		boardID, err := helper.IDParam(r, "boardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		query := r.URL.Query()
		filters := model.BoardArchiveFilters{
			Title:       query.Get("title"),
			Description: query.Get("description"),
			DateFrom:    query.Get("dateFrom"),
			DateTo:      query.Get("dateTo"),
			Page:        parsePositiveInt(query.Get("page"), 1),
			Limit:       10,
		}

		res, err := h.service.GetBoardArchive(r.Context(), projectID, boardID, filters)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func parsePositiveInt(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}
