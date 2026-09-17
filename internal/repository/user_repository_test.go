package repository

import (
	"testing"

	"go_kanban_service/internal/model"
)

func TestMissingUserIDs(t *testing.T) {
	users := func(ids ...int64) []model.User {
		out := make([]model.User, 0, len(ids))
		for _, id := range ids {
			out = append(out, model.User{ID: id})
		}
		return out
	}

	cases := []struct {
		name  string
		ids   []int64
		found []model.User
		want  []int64
	}{
		{
			// Ровно тот случай, на котором ломалось прежнее len(users) < len(ids):
			// сто комментариев от двух авторов, оба на месте — недостающих нет.
			name:  "повторы при полном совпадении",
			ids:   []int64{7, 7, 7, 9, 7, 9},
			found: users(7, 9),
			want:  nil,
		},
		{
			name:  "недостающий отдаётся один раз, без дублей",
			ids:   []int64{7, 42, 7, 42, 9},
			found: users(7, 9),
			want:  []int64{42},
		},
		{
			name:  "не нашли никого",
			ids:   []int64{1, 1, 2},
			found: nil,
			want:  []int64{1, 2},
		},
		{
			name:  "лишние в выдаче не мешают",
			ids:   []int64{5},
			found: users(5, 6),
			want:  nil,
		},
	}

	for _, c := range cases {
		got := missingUserIDs(c.ids, c.found)
		if len(got) != len(c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}
