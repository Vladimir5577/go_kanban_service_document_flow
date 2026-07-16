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

type CardHandler struct {
	service service.CardServiceInterface
}

func NewCardHandler(s service.CardServiceInterface) *CardHandler {
	return &CardHandler{service: s}
}

func (h *CardHandler) CreateCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req dto.CreateCardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, map[validationCodeKey]apperr.ErrorCode{
				{Field: "Title", Tag: "required"}:    apperr.CodeColumnIDAndTitleRequired,
				{Field: "ColumnID", Tag: "required"}: apperr.CodeColumnIDAndTitleRequired,
			}))
			return
		}

		created, err := h.service.CreateCard(r.Context(), req)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		// Return full enriched card (with boardId, createdBy, etc.)
		detail, err := h.service.GetCardDetail(r.Context(), created.ID)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusCreated, detail)
	}
}

func (h *CardHandler) GetCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		res, err := h.service.GetCardDetail(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, res)
	}
}

func (h *CardHandler) UpdateCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var req dto.UpdateCardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}
		if err := validator.Validate.Struct(req); err != nil {
			helper.WriteError(w, validationError(err, nil))
			return
		}

		if _, err = h.service.UpdateCard(r.Context(), id, req); err != nil {
			helper.WriteError(w, err)
			return
		}
		// Return full enriched card (with boardId, createdBy, etc.)
		detail, err := h.service.GetCardDetail(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, detail)
	}
}

func (h *CardHandler) DeleteCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.DeleteCard(r.Context(), id); err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusNoContent, nil)
	}
}

func (h *CardHandler) UpdateAssignees() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var payload struct {
			UserIDs []int64 `json:"user_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		if err := h.service.UpdateAssignees(r.Context(), id, payload.UserIDs); err != nil {
			helper.WriteError(w, err)
			return
		}

		cardDetail, err := h.service.GetCardDetail(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"assignees": cardDetail.Assignees,
		})
	}
}

func (h *CardHandler) MoveCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		var payload struct {
			ColumnID int64   `json:"column_id"`
			Position float64 `json:"position"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			helper.WriteError(w, invalidJSONError())
			return
		}

		if _, err = h.service.MoveCard(r.Context(), id, payload.ColumnID, payload.Position); err != nil {
			helper.WriteError(w, err)
			return
		}
		// Return full enriched card (with boardId, createdBy, etc.)
		detail, err := h.service.GetCardDetail(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, detail)
	}
}

func (h *CardHandler) ArchiveCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if err := h.service.ArchiveCard(r.Context(), id); err != nil {
			helper.WriteError(w, err)
			return
		}

		cardDetail, err := h.service.GetCardDetail(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"id":         id,
			"isArchived": cardDetail.IsArchived,
		})
	}
}

func (h *CardHandler) CompleteCard() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := helper.IDParam(r, "id")
		if err != nil {
			helper.WriteError(w, err)
			return
		}

		if _, err = h.service.CompleteCard(r.Context(), id); err != nil {
			helper.WriteError(w, err)
			return
		}
		// Return full enriched card (with boardId, createdBy, etc.)
		detail, err := h.service.GetCardDetail(r.Context(), id)
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, detail)
	}
}
