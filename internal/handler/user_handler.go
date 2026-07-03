package handler

import (
	"net/http"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/service"
)

type UserHandler struct {
	service service.UserServiceInterface
}

func NewUserHandler(s service.UserServiceInterface) *UserHandler {
	return &UserHandler{service: s}
}

func (h *UserHandler) LoginCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		res, err := h.service.LoginCheck(r.Context())
		if err != nil {
			helper.WriteError(w, err)
			return
		}
		helper.WriteJSON(w, http.StatusOK, dto.MapUserResponse(res))
	}
}
