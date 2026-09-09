package handler

import (
	"net/http"
	"strconv"
	"strings"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

type HistoryHandler struct {
	service service.HistoryServiceInterface
}

func NewHistoryHandler(s service.HistoryServiceInterface) *HistoryHandler {
	return &HistoryHandler{service: s}
}

func (h *HistoryHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		cursor, userID, title, limit := parseHistoryListQuery(r)
		page, err := h.service.List(r.Context(), projectID, cursor, limit, userID, title)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapHistoryList(page.Items, page.HasMore, page.NextCursor))
	}
}

func (h *HistoryHandler) ListByCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		cursor, userID, title, limit := parseHistoryListQuery(r)
		page, err := h.service.ListByCard(r.Context(), cardID, cursor, limit, userID, title)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapHistoryList(page.Items, page.HasMore, page.NextCursor))
	}
}

func (h *HistoryHandler) Undo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		action, entityTitle, err := h.service.Undo(r.Context(), projectID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.HistoryUndoResponse{Action: action, EntityTitle: entityTitle})
	}
}

func parseHistoryListQuery(r *http.Request) (cursor, userID int64, title string, limit int32) {
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, _ = strconv.ParseInt(raw, 10, 64)
	}
	if raw := r.URL.Query().Get("userId"); raw != "" {
		userID, _ = strconv.ParseInt(raw, 10, 64)
	}
	title = strings.TrimSpace(r.URL.Query().Get("title"))
	limit = int32(parsePositiveInt(r.URL.Query().Get("limit"), 50))
	return
}
