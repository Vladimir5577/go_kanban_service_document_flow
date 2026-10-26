package service

import (
	"encoding/json"
	"testing"
	"time"

	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

func TestMapTaskListItemsEmpty(t *testing.T) {
	got := mapTaskListItems(nil)
	if got == nil {
		t.Fatalf("result is nil, want empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestMapTaskListItemsFlatAndParent(t *testing.T) {
	priority := "high"
	due := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	parentID := int64(555)
	parentTitle := "Parent"
	created := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	estimate := 480

	got := mapTaskListItems([]repository.AssignedCardRow{
		{
			ProjectID:       1,
			ProjectName:     "Project A",
			BoardID:         10,
			BoardTitle:      "Board",
			ColumnID:        100,
			ColumnTitle:     "Todo",
			CardID:          1000,
			CardTitle:       "Card 1",
			Priority:        &priority,
			DueDate:         &due,
			CreatedAt:       created,
			EstimateMinutes: &estimate,
		},
		{
			ProjectID:   1,
			ProjectName: "Project A",
			BoardID:     10,
			BoardTitle:  "Board",
			ColumnID:    100,
			ColumnTitle: "Todo",
			CardID:      1001,
			CardTitle:   "Child",
			ParentID:    &parentID,
			ParentTitle: &parentTitle,
			CreatedAt:   created,
		},
	})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != 1000 || got[0].ParentID != nil {
		t.Fatalf("root = %#v", got[0])
	}
	if got[1].ParentID == nil || *got[1].ParentID != parentID || got[1].ParentTitle == nil || *got[1].ParentTitle != parentTitle {
		t.Fatalf("child = %#v", got[1])
	}
	if got[0].Column.Title != "Todo" || got[0].Project.Name != "Project A" {
		t.Fatalf("refs = %#v", got[0])
	}
	if got[0].EstimateMinutes == nil || *got[0].EstimateMinutes != estimate || got[1].EstimateMinutes != nil {
		t.Fatalf("estimate = %#v / %#v", got[0].EstimateMinutes, got[1].EstimateMinutes)
	}

	body, err := json.Marshal(got[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !json.Valid(body) {
		t.Fatalf("invalid json: %s", body)
	}
}

func TestAttachAssigneesToTaskItems(t *testing.T) {
	items := []*dto.TaskListItem{{ID: 1}, {ID: 2}, {ID: 3}}
	attachAssigneesToTaskItems(items, map[int64][]int64{
		1: {10, 11},
		2: {99},
	}, []model.User{
		{ID: 10, Login: "ivan", Lastname: "Иванов", Firstname: "Иван"},
		{ID: 11, Login: "petr", Lastname: "Петров", Firstname: "Пётр"},
	})

	if len(items[0].Assignees) != 2 || items[0].Assignees[0].ID != 10 || items[0].Assignees[1].ID != 11 || items[0].Assignees[0].Lastname != "Иванов" {
		t.Fatalf("item 1 = %#v", items[0].Assignees)
	}
	if len(items[1].Assignees) != 0 {
		t.Fatalf("missing user should stay empty, got %#v", items[1].Assignees)
	}
	if items[2].Assignees != nil {
		t.Fatalf("no assignee should stay nil, got %#v", items[2].Assignees)
	}
}
