package validator

import (
	"github.com/go-playground/validator/v10"
)

var Validate *validator.Validate

// Init инициализирует глобальный валидатор
func Init() {
	Validate = validator.New(validator.WithRequiredStructEnabled())
}
