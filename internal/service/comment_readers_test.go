package service

import (
	"testing"
	"time"

	"go_kanban_service/internal/model"
)

func TestBuildCommentReaders(t *testing.T) {
	users := []model.User{
		{ID: 1, Lastname: "Борисов"},
		{ID: 3, Lastname: "Васильев"},
	}
	early := time.Date(2026, 9, 10, 14, 32, 0, 0, time.UTC)
	late := time.Date(2026, 9, 12, 9, 15, 0, 0, time.UTC)
	edited := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	readAt := map[int64]time.Time{1: late, 3: early}

	t.Run("порядок по времени прочтения, а не по выдаче пользователей", func(t *testing.T) {
		readers := buildCommentReaders(users, readAt, nil)

		if len(readers) != 2 || readers[0].User.ID != 3 || readers[1].User.ID != 1 {
			t.Fatalf("readers = %+v, want [3 1] по возрастанию времени", readers)
		}
	})

	t.Run("правка после прочтения взводит флаг только у тех, кто читал раньше", func(t *testing.T) {
		readers := buildCommentReaders(users, readAt, &edited)

		// Васильев читал 10-го, правка 11-го — он видел другой текст.
		if !readers[0].ReadBeforeEdit {
			t.Errorf("Васильев: ReadBeforeEdit = false, хотя правка позже прочтения")
		}
		// Борисов читал 12-го, уже после правки.
		if readers[1].ReadBeforeEdit {
			t.Errorf("Борисов: ReadBeforeEdit = true, хотя читал после правки")
		}
	})

	t.Run("человек без отметки в выдачу не попадает", func(t *testing.T) {
		// Так выпадает автор: его id отфильтрован до гидрации, но если
		// GetUsersByIDs вернёт лишнего — он не должен просочиться в ответ.
		withStranger := append(users, model.User{ID: 42, Lastname: "Чужой"})

		readers := buildCommentReaders(withStranger, readAt, nil)

		if len(readers) != 2 {
			t.Fatalf("readers = %+v, want только двоих с отметками", readers)
		}
	})

	t.Run("никто не читал — пустой список, а не nil", func(t *testing.T) {
		readers := buildCommentReaders(users, map[int64]time.Time{}, nil)

		if readers == nil || len(readers) != 0 {
			t.Fatalf("readers = %+v, want []", readers)
		}
	})
}
