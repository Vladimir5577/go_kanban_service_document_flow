package handler

import (
	"net/http"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

type ActivityHandler struct {
	service service.ActivityServiceInterface
}

func NewActivityHandler(s service.ActivityServiceInterface) *ActivityHandler {
	return &ActivityHandler{service: s}
}

func (h *ActivityHandler) GetActivities() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cardID, err := helper.IDParam(r, "cardId")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		activities, err := h.service.GetActivities(r.Context(), cardID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"items":      dto.MapActivitiesResponse(activities),
			"hasMore":    false,
			"nextOffset": 0,
		})
	}
}
