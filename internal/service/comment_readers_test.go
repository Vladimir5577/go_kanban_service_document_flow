package service

import (
	"testing"
	"time"

	"go_kanban_service/internal/model"
)

func TestCommentAudienceIDs(t *testing.T) {
	members := []model.ProjectUser{{UserID: 7}, {UserID: 3}, {UserID: 1}}

	tests := []struct {
		name     string
		ownerID  int64
		authorID int64
		want     []int64
	}{
		{name: "владелец идёт первым и не дублируется участником", ownerID: 1, authorID: 99, want: []int64{1, 7, 3}},
		{name: "автор выпадает из списка", ownerID: 1, authorID: 7, want: []int64{1, 3}},
		{name: "автор-владелец тоже выпадает", ownerID: 1, authorID: 1, want: []int64{7, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commentAudienceIDs(tt.ownerID, members, tt.authorID)
			if len(got) != len(tt.want) {
				t.Fatalf("commentAudienceIDs() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("commentAudienceIDs() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestSplitCommentReaders(t *testing.T) {
	users := []model.User{
		{ID: 1, Lastname: "Борисов"},
		{ID: 2, Lastname: "Антонов"},
		{ID: 3, Lastname: "Васильев"},
	}
	early := time.Date(2026, 9, 10, 14, 32, 0, 0, time.UTC)
	late := time.Date(2026, 9, 12, 9, 15, 0, 0, time.UTC)
	edited := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	readAt := map[int64]time.Time{1: late, 3: early}

	t.Run("прочитавшие идут по времени, остальные по фамилии", func(t *testing.T) {
		res := splitCommentReaders(users, readAt, nil)

		if len(res.Readers) != 2 || res.Readers[0].User.ID != 3 || res.Readers[1].User.ID != 1 {
			t.Fatalf("Readers = %+v, want [3 1] по возрастанию времени", res.Readers)
		}
		if len(res.Pending) != 1 || res.Pending[0].ID != 2 {
			t.Fatalf("Pending = %+v, want [2]", res.Pending)
		}
	})

	t.Run("правка после прочтения взводит флаг только у тех, кто читал раньше", func(t *testing.T) {
		res := splitCommentReaders(users, readAt, &edited)

		// Васильев читал 10-го, правка 11-го — он видел другой текст.
		if !res.Readers[0].ReadBeforeEdit {
			t.Errorf("Васильев: ReadBeforeEdit = false, хотя правка позже прочтения")
		}
		// Борисов читал 12-го, уже после правки.
		if res.Readers[1].ReadBeforeEdit {
			t.Errorf("Борисов: ReadBeforeEdit = true, хотя читал после правки")
		}
	})

	t.Run("никто не читал — все в ожидании", func(t *testing.T) {
		res := splitCommentReaders(users, map[int64]time.Time{}, nil)

		if len(res.Readers) != 0 {
			t.Fatalf("Readers = %+v, want пусто", res.Readers)
		}
		if len(res.Pending) != 3 || res.Pending[0].Lastname != "Антонов" {
			t.Fatalf("Pending = %+v, want три человека по алфавиту", res.Pending)
		}
	})
}
