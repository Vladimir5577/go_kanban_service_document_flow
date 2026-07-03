// Package apperr определяет доменные ошибки, независимые от транспорта (HTTP).
//
// Слои repository и service возвращают эти ошибки (через errors.Is / оборачивание
// в %w), а слой handler мапит их в HTTP-статусы функцией helper.WriteError.
// Так бизнес-логика не знает про HTTP, а handler не угадывает статус по тексту.
package apperr

type ErrorCode string

const (
	CodeNotFound     ErrorCode = "not_found"
	CodeForbidden    ErrorCode = "forbidden"
	CodeUnauthorized ErrorCode = "unauthorized"
	CodeConflict     ErrorCode = "conflict"
	CodeValidation   ErrorCode = "validation_failed"
)

type Error struct {
	Code    ErrorCode
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Err
}

func New(code ErrorCode, msg string) *Error {
	return &Error{
		Code:    code,
		Message: msg,
	}
}

var (
	// ErrNotFound — запрошенная сущность не существует (или soft-deleted). → 404
	ErrNotFound = New(CodeNotFound, "not found")

	// ErrForbidden — пользователь аутентифицирован, но не имеет прав на ресурс. → 403
	ErrForbidden = New(CodeForbidden, "forbidden")

	// ErrUnauthorized — пользователь не аутентифицирован. → 401
	ErrUnauthorized = New(CodeUnauthorized, "unauthorized")

	// ErrConflict — нарушение инварианта/уникальности (дубликат и т.п.). → 409
	ErrConflict = New(CodeConflict, "conflict")

	// ErrValidation — некорректные входные данные запроса. → 400
	ErrValidation = New(CodeValidation, "validation failed")
)
