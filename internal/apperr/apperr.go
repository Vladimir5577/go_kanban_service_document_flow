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

	CodeAccessDenied                ErrorCode = "access_denied"
	CodeAttachmentLimitReached      ErrorCode = "attachment_limit_reached"
	CodeAttachmentNotFound          ErrorCode = "attachment_not_found"
	CodeAttachmentNotPreviewable    ErrorCode = "attachment_not_previewable"
	CodeBoardCardLimitReached       ErrorCode = "board_card_limit_reached"
	CodeBoardHasCards               ErrorCode = "board_has_cards"
	CodeBoardHasNoProject           ErrorCode = "board_has_no_project"
	CodeBoardNotFound               ErrorCode = "board_not_found"
	CodeBoardTitleRequired          ErrorCode = "board_title_required"
	CodeBoardTitleTooLong           ErrorCode = "board_title_too_long"
	CodeCannotRemoveOwner           ErrorCode = "cannot_remove_owner"
	CodeCannotRemoveSelf            ErrorCode = "cannot_remove_self"
	CodeCardNotFound                ErrorCode = "card_not_found"
	CodeColumnHasCards              ErrorCode = "column_has_cards"
	CodeColumnIDAndPositionRequired ErrorCode = "column_id_and_position_required"
	CodeColumnIDAndTitleRequired    ErrorCode = "column_id_and_title_required"
	CodeColumnNotFound              ErrorCode = "column_not_found"
	CodeColumnTitleRequired         ErrorCode = "column_title_required"
	CodeCommentAuthorOnly           ErrorCode = "comment_author_only"
	CodeCommentBodyRequired         ErrorCode = "comment_body_required"
	CodeCommentBodyTooLong          ErrorCode = "comment_body_too_long"
	CodeCommentLimitReached         ErrorCode = "comment_limit_reached"
	CodeCommentNotFound             ErrorCode = "comment_not_found"
	CodeDescriptionInvalidType      ErrorCode = "description_invalid_type"
	CodeFileNotFoundOnDisk          ErrorCode = "file_not_found_on_disk"
	CodeFileNotProvided             ErrorCode = "file_not_provided"
	CodeFolderNameRequired          ErrorCode = "folder_name_required"
	CodeFolderNameTooLong           ErrorCode = "folder_name_too_long"
	CodeFolderNotFound              ErrorCode = "folder_not_found"
	CodeInsufficientPermissions     ErrorCode = "insufficient_permissions"
	CodeInvalidJSON                 ErrorCode = "invalid_json"
	CodeInvalidRole                 ErrorCode = "invalid_role"
	CodeInvalidRoleForUser          ErrorCode = "invalid_role_for_user"
	CodeLabelNameRequired           ErrorCode = "label_name_required"
	CodeLabelNotFound               ErrorCode = "label_not_found"
	CodeMemberNotFound              ErrorCode = "member_not_found"
	CodeMembersArrayExpected        ErrorCode = "members_array_expected"
	CodeMembersListEmpty            ErrorCode = "members_list_empty"
	CodeOwnerRoleImmutable          ErrorCode = "owner_role_immutable"
	CodeProjectAccessDenied         ErrorCode = "project_access_denied"
	CodeProjectCreateFailed         ErrorCode = "project_create_failed"
	CodeProjectNameRequired         ErrorCode = "project_name_required"
	CodeProjectNameTooLong          ErrorCode = "project_name_too_long"
	CodeProjectNotFound             ErrorCode = "project_not_found"
	CodeSubtaskNotFound             ErrorCode = "subtask_not_found"
	CodeSubtaskTitleRequired        ErrorCode = "subtask_title_required"
	CodeUpdateFieldsRequired        ErrorCode = "update_fields_required"
	CodeUserNotFound                ErrorCode = "user_not_found"
	CodeUserNotProjectMember        ErrorCode = "user_not_project_member"
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

func (e *Error) Is(target error) bool {
	targetErr, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Code == targetErr.Code || categoryCode(e.Code) == targetErr.Code
}

func New(code ErrorCode, msg string) *Error {
	return &Error{
		Code:    code,
		Message: msg,
	}
}

func categoryCode(code ErrorCode) ErrorCode {
	switch code {
	case CodeAttachmentNotFound,
		CodeAttachmentNotPreviewable,
		CodeBoardNotFound,
		CodeCardNotFound,
		CodeColumnNotFound,
		CodeCommentNotFound,
		CodeFileNotFoundOnDisk,
		CodeFolderNotFound,
		CodeLabelNotFound,
		CodeMemberNotFound,
		CodeProjectNotFound,
		CodeSubtaskNotFound,
		CodeUserNotFound:
		return CodeNotFound
	case CodeAccessDenied,
		CodeCommentAuthorOnly,
		CodeInsufficientPermissions,
		CodeProjectAccessDenied:
		return CodeForbidden
	case CodeBoardCardLimitReached,
		CodeBoardHasCards,
		CodeColumnHasCards,
		CodeCommentLimitReached:
		return CodeConflict
	case CodeAttachmentLimitReached,
		CodeBoardHasNoProject,
		CodeBoardTitleRequired,
		CodeBoardTitleTooLong,
		CodeCannotRemoveOwner,
		CodeCannotRemoveSelf,
		CodeColumnIDAndPositionRequired,
		CodeColumnIDAndTitleRequired,
		CodeColumnTitleRequired,
		CodeCommentBodyRequired,
		CodeCommentBodyTooLong,
		CodeDescriptionInvalidType,
		CodeFileNotProvided,
		CodeFolderNameRequired,
		CodeFolderNameTooLong,
		CodeInvalidJSON,
		CodeInvalidRole,
		CodeInvalidRoleForUser,
		CodeLabelNameRequired,
		CodeMembersArrayExpected,
		CodeMembersListEmpty,
		CodeOwnerRoleImmutable,
		CodeProjectNameRequired,
		CodeProjectNameTooLong,
		CodeSubtaskTitleRequired,
		CodeUpdateFieldsRequired,
		CodeUserNotProjectMember:
		return CodeValidation
	default:
		return code
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
