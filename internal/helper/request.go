package helper

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"go_kanban_service/internal/apperr"
)

// IDParam читает числовой параметр пути (например {id}) и возвращает его как int64.
// При отсутствии или нечисловом значении возвращает apperr.ErrValidation.
func IDParam(r *http.Request, name string) (int64, error) {
	raw := chi.URLParam(r, name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: invalid %s", apperr.ErrValidation, name)
	}
	return id, nil
}
