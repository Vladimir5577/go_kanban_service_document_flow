package dto

import (
	"testing"

	"go_kanban_service/internal/model"
)

func TestUserDisplayName(t *testing.T) {
	patronymic := "Петрович"

	tests := []struct {
		name string
		user model.User
		want string
	}{
		{
			name: "ignores patronymic",
			user: model.User{Lastname: "Иванов", Firstname: "Иван", Patronymic: &patronymic},
			want: "Иванов Иван",
		},
		{
			name: "without patronymic",
			user: model.User{Lastname: "Иванов", Firstname: "Иван"},
			want: "Иванов Иван",
		},
		{
			name: "skips blank parts",
			user: model.User{Firstname: "Иван", Patronymic: &patronymic},
			want: "Иван",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserDisplayName(tt.user); got != tt.want {
				t.Fatalf("UserDisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}
