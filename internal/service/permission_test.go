package service

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/model"
)

func member(role Role) *model.ProjectUser {
	return &model.ProjectUser{UserID: 7, Role: string(role)}
}

func TestResolveRole(t *testing.T) {
	tests := []struct {
		name    string
		userID  int64
		ownerID int64
		member  *model.ProjectUser
		want    Role
		denied  bool
	}{
		{name: "владелец всегда админ", userID: 1, ownerID: 1, member: nil, want: RoleAdmin},
		{name: "владелец админ, даже если числится вьюером", userID: 1, ownerID: 1, member: member(RoleViewer), want: RoleAdmin},
		{name: "участник получает свою роль", userID: 7, ownerID: 1, member: member(RoleEditor), want: RoleEditor},
		{name: "не участник — отказ", userID: 7, ownerID: 1, member: nil, denied: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRole(tt.userID, tt.ownerID, tt.member)

			if tt.denied {
				var appErr *apperr.Error
				if !errors.As(err, &appErr) || appErr.Code != apperr.CodeAccessDenied {
					t.Fatalf("err = %v, want %s", err, apperr.CodeAccessDenied)
				}
				return
			}

			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("role = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasRole(t *testing.T) {
	tests := []struct {
		name     string
		userRole Role
		minRole  Role
		want     bool
	}{
		{name: "вьюер проходит на чтение", userRole: RoleViewer, minRole: RoleViewer, want: true},
		{name: "вьюер не проходит на правку", userRole: RoleViewer, minRole: RoleEditor},
		{name: "админ проходит везде", userRole: RoleAdmin, minRole: RoleEditor, want: true},
		{name: "редактор не проходит на админское", userRole: RoleEditor, minRole: RoleAdmin},
		{name: "неизвестная роль пользователя — отказ", userRole: Role("KANBAN_GOD"), minRole: RoleViewer},
		{name: "неизвестное требование — отказ", userRole: RoleAdmin, minRole: Role("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasRole(tt.userRole, tt.minRole); got != tt.want {
				t.Fatalf("hasRole(%q, %q) = %v, want %v", tt.userRole, tt.minRole, got, tt.want)
			}
		})
	}
}

// checkContextRole — общая проверка RequireCardRole и RequireColumnRole: ошибка
// в ней открывает или закрывает доступ сразу ко всем операциям с карточками.
func TestCheckContextRole(t *testing.T) {
	role := func(r Role) pgtype.Text { return pgtype.Text{String: string(r), Valid: true} }
	deleted := pgtype.Timestamptz{Time: time.Now(), Valid: true}

	tests := []struct {
		name     string
		userID   int64
		member   pgtype.Text
		deleted  pgtype.Timestamptz
		minRole  Role
		want     Role
		wantCode apperr.ErrorCode
	}{
		{name: "владелец без членства — админ", userID: 1, minRole: RoleAdmin, want: RoleAdmin},
		{name: "редактор проходит на редактора", userID: 7, member: role(RoleEditor), minRole: RoleEditor, want: RoleEditor},
		{name: "вьюер не проходит на редактора", userID: 7, member: role(RoleViewer), minRole: RoleEditor, wantCode: apperr.CodeAccessDenied},
		{name: "не участник — отказ", userID: 7, minRole: RoleViewer, wantCode: apperr.CodeAccessDenied},
		{name: "удалённый проект важнее прав", userID: 1, deleted: deleted, minRole: RoleViewer, wantCode: apperr.CodeProjectNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkContextRole(tt.userID, 1, tt.member, tt.deleted, tt.minRole)
			if tt.wantCode != "" {
				var appErr *apperr.Error
				if !errors.As(err, &appErr) || appErr.Code != tt.wantCode {
					t.Fatalf("err = %v, want %s", err, tt.wantCode)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}
