package repository

import (
	"strings"
	"testing"
	"time"
)

func taskListSQL(t *testing.T, f TaskListParams) (string, []any) {
	t.Helper()
	sqlStr, args, err := taskListQuery(f).ToSql()
	if err != nil {
		t.Fatalf("ToSql: %v", err)
	}
	return sqlStr, args
}

// Смысл динамической сборки в том, что неактивный фильтр не попадает в SQL
// вообще: иначе планировщик снова получит один план на все комбинации.
func TestTaskListQueryOmitsInactiveFilters(t *testing.T) {
	sqlStr, args := taskListSQL(t, TaskListParams{ViewerID: 7, Sort: "priority", SortDesc: true})

	// Иглы подобраны так, чтобы не совпадать со списком колонок в SELECT.
	for _, unwanted := range []string{
		"ILIKE", "kanban_card_assignee", "c.priority =", "c.created_by_id",
		"c.due_date >=", "c.due_date <", "c.completed_at IS", "c.parent_id IS NOT NULL",
	} {
		if strings.Contains(sqlStr, unwanted) {
			t.Errorf("пустой фильтр просочился в SQL: %q\n%s", unwanted, sqlStr)
		}
	}
	// Только два плейсхолдера видимости.
	if len(args) != 2 || args[0] != int64(7) || args[1] != int64(7) {
		t.Errorf("args = %v, want [7 7]", args)
	}
}

// Главный выигрыш правки: assignee_id становится JOIN и может зайти
// с idx_kanban_card_assignee_user_id вместо EXISTS поверх скана карточек.
func TestTaskListQueryAssigneeBecomesJoin(t *testing.T) {
	sqlStr, args := taskListSQL(t, TaskListParams{ViewerID: 7, AssigneeID: 42})

	if !strings.Contains(sqlStr, "JOIN kanban_card_assignee ca ON ca.card_id = c.id") {
		t.Fatalf("нет JOIN по исполнителю:\n%s", sqlStr)
	}
	if strings.Contains(sqlStr, "EXISTS (SELECT 1 FROM kanban_card_assignee") {
		t.Errorf("остался EXISTS вместо JOIN:\n%s", sqlStr)
	}
	if len(args) != 3 || args[2] != int64(42) {
		t.Errorf("args = %v, want последним 42", args)
	}
}

func TestTaskListQueryAssigneeNoneStaysNotExists(t *testing.T) {
	sqlStr, args := taskListSQL(t, TaskListParams{ViewerID: 7, AssigneeID: -1})

	if !strings.Contains(sqlStr, "NOT EXISTS (SELECT 1 FROM kanban_card_assignee") {
		t.Fatalf("нет NOT EXISTS для «без исполнителя»:\n%s", sqlStr)
	}
	if strings.Contains(sqlStr, "JOIN kanban_card_assignee") {
		t.Errorf("JOIN не должен появляться для «без исполнителя»:\n%s", sqlStr)
	}
	if len(args) != 2 {
		t.Errorf("args = %v, лишний плейсхолдер", args)
	}
}

func TestTaskListQueryDateRangesOnlyWhenSet(t *testing.T) {
	from := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)

	sqlStr, args := taskListSQL(t, TaskListParams{ViewerID: 7, CreatedFrom: &from, ArchivedTo: &to})

	if !strings.Contains(sqlStr, "c.created_at >= $3") {
		t.Errorf("нет нижней границы created_at:\n%s", sqlStr)
	}
	if strings.Contains(sqlStr, "c.created_at <") {
		t.Errorf("верхняя граница created_at не задавалась, но попала в SQL:\n%s", sqlStr)
	}
	if !strings.Contains(sqlStr, "END) < $4") {
		t.Errorf("нет верхней границы archived_at:\n%s", sqlStr)
	}
	if len(args) != 4 || args[2] != from || args[3] != to {
		t.Errorf("args = %v", args)
	}
}

// sort приходит из запроса пользователя и не должен попадать в SQL как текст.
func TestTaskListOrderByIsWhitelisted(t *testing.T) {
	cases := map[string]string{
		"title":       "c.title DESC",
		"createdAt":   "c.created_at DESC",
		"completedAt": "c.completed_at DESC NULLS LAST",
		"priority":    "CASE c.priority WHEN 'high'",
		"c.id; DROP":  "CASE c.priority WHEN 'high'", // мусор уходит в ветку по умолчанию
	}
	for sort, want := range cases {
		got := taskListOrderBy(sort, true)
		if len(got) != 2 || !strings.HasPrefix(got[0], want) {
			t.Errorf("sort=%q -> %v, want первым %q", sort, got, want)
		}
		if got[len(got)-1] != "c.id ASC" {
			t.Errorf("sort=%q: нет стабильного тайбрейка c.id ASC: %v", sort, got)
		}
	}

	sqlStr, _ := taskListSQL(t, TaskListParams{ViewerID: 7, Sort: "c.id; DROP TABLE kanban_card"})
	if strings.Contains(sqlStr, "DROP") {
		t.Fatalf("значение sort утекло в SQL:\n%s", sqlStr)
	}
}

// Подстрока едет параметром, а не конкатенацией в текст запроса.
func TestTaskListQueryTitleSearchIsParameterised(t *testing.T) {
	sqlStr, args := taskListSQL(t, TaskListParams{ViewerID: 7, TitleQuery: `100\% готово`})

	if !strings.Contains(sqlStr, `c.title ILIKE '%' || $3 || '%' ESCAPE '\'`) {
		t.Fatalf("ожидался параметризованный ILIKE с ESCAPE:\n%s", sqlStr)
	}
	if len(args) != 3 || args[2] != `100\% готово` {
		t.Errorf("args = %v", args)
	}
}
