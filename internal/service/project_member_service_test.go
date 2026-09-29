package service

import (
	"testing"

	"go_kanban_service/internal/model"
)

// Пустой записи в истории быть не должно: при неизменном составе
// ReplaceMembers выходит раньше записи, при любом отличии — пишет.
func TestSameMembers(t *testing.T) {
	folder := int64(3)
	before := []model.ProjectUser{
		{UserID: 1, Role: "KANBAN_ADMIN"},
		{UserID: 2, Role: "KANBAN_EDITOR"},
	}
	tests := []struct {
		name  string
		after []model.ProjectUser
		want  bool
	}{
		{"тот же состав в другом порядке, папка не в счёт", []model.ProjectUser{
			{UserID: 2, Role: "KANBAN_EDITOR", FolderID: &folder}, {UserID: 1, Role: "KANBAN_ADMIN"},
		}, true},
		{"сменили роль", []model.ProjectUser{
			{UserID: 1, Role: "KANBAN_ADMIN"}, {UserID: 2, Role: "KANBAN_VIEWER"},
		}, false},
		{"добавили", []model.ProjectUser{
			{UserID: 1, Role: "KANBAN_ADMIN"}, {UserID: 2, Role: "KANBAN_EDITOR"}, {UserID: 3, Role: "KANBAN_EDITOR"},
		}, false},
		{"убрали", []model.ProjectUser{{UserID: 1, Role: "KANBAN_ADMIN"}}, false},
		{"заменили одного другим", []model.ProjectUser{
			{UserID: 1, Role: "KANBAN_ADMIN"}, {UserID: 3, Role: "KANBAN_EDITOR"},
		}, false},
	}
	for _, tt := range tests {
		if got := sameMembers(before, tt.after); got != tt.want {
			t.Errorf("%s: sameMembers = %v, want %v", tt.name, got, tt.want)
		}
	}
}
