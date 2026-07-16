package service

import (
	"encoding/json"
	"testing"
	"time"

	"go_kanban_service/internal/repository"
)

func TestBuildAssignedTreeEmpty(t *testing.T) {
	got := buildAssignedTree(nil)
	if got == nil {
		t.Fatalf("result is nil, want empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestBuildAssignedTreeGroupsAndPreservesOrder(t *testing.T) {
	priority := "high"
	due := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	color := "danger"

	got := buildAssignedTree([]repository.AssignedCardRow{
		{
			ProjectID:   1,
			ProjectName: "Project A",
			BoardID:     10,
			BoardTitle:  "Board 1",
			ColumnID:    100,
			ColumnTitle: "Todo",
			CardID:      1000,
			CardTitle:   "Card 1",
			Priority:    &priority,
			DueDate:     &due,
			BorderColor: &color,
		},
		{
			ProjectID:   1,
			ProjectName: "Project A",
			BoardID:     10,
			BoardTitle:  "Board 1",
			ColumnID:    100,
			ColumnTitle: "Todo",
			CardID:      1001,
			CardTitle:   "Card 2",
		},
		{
			ProjectID:   1,
			ProjectName: "Project A",
			BoardID:     10,
			BoardTitle:  "Board 1",
			ColumnID:    101,
			ColumnTitle: "Doing",
			CardID:      1002,
			CardTitle:   "Card 3",
		},
		{
			ProjectID:   1,
			ProjectName: "Project A",
			BoardID:     11,
			BoardTitle:  "Board 2",
			ColumnID:    110,
			ColumnTitle: "Todo",
			CardID:      1003,
			CardTitle:   "Card 4",
		},
		{
			ProjectID:   2,
			ProjectName: "Project B",
			BoardID:     20,
			BoardTitle:  "Board 1",
			ColumnID:    200,
			ColumnTitle: "Todo",
			CardID:      2000,
			CardTitle:   "Card 5",
		},
	})

	if len(got) != 2 {
		t.Fatalf("projects len = %d, want 2", len(got))
	}
	if got[0].ID != 1 || got[0].Name != "Project A" {
		t.Fatalf("first project = %#v", got[0])
	}
	if len(got[0].Boards) != 2 {
		t.Fatalf("project A boards len = %d, want 2", len(got[0].Boards))
	}
	if len(got[0].Boards[0].Columns) != 2 {
		t.Fatalf("board 1 columns len = %d, want 2", len(got[0].Boards[0].Columns))
	}
	if len(got[0].Boards[0].Columns[0].Cards) != 2 {
		t.Fatalf("first column cards len = %d, want 2", len(got[0].Boards[0].Columns[0].Cards))
	}
	if got[0].Boards[0].Columns[0].Cards[0].Title != "Card 1" ||
		got[0].Boards[0].Columns[0].Cards[1].Title != "Card 2" {
		t.Fatalf("card order = %#v", got[0].Boards[0].Columns[0].Cards)
	}
	if got[0].Boards[0].Columns[1].Cards[0].Title != "Card 3" {
		t.Fatalf("second column first card = %#v", got[0].Boards[0].Columns[1].Cards[0])
	}
	if got[0].Boards[1].Columns[0].Cards[0].Title != "Card 4" {
		t.Fatalf("second board first card = %#v", got[0].Boards[1].Columns[0].Cards[0])
	}
	if got[1].Boards[0].Columns[0].Cards[0].Title != "Card 5" {
		t.Fatalf("project B first card = %#v", got[1].Boards[0].Columns[0].Cards[0])
	}

	firstCard := got[0].Boards[0].Columns[0].Cards[0]
	if firstCard.Priority == nil || *firstCard.Priority != priority {
		t.Fatalf("priority = %#v, want %q", firstCard.Priority, priority)
	}
	if firstCard.DueDate == nil || !firstCard.DueDate.Equal(due) {
		t.Fatalf("due date = %#v, want %s", firstCard.DueDate, due)
	}
	if firstCard.BorderColor == nil || *firstCard.BorderColor != color {
		t.Fatalf("border color = %#v, want %q", firstCard.BorderColor, color)
	}
}

func TestBuildAssignedTreeNullableFieldsMarshalAsNull(t *testing.T) {
	got := buildAssignedTree([]repository.AssignedCardRow{
		{
			ProjectID:   1,
			ProjectName: "Project A",
			BoardID:     10,
			BoardTitle:  "Board 1",
			ColumnID:    100,
			ColumnTitle: "Todo",
			CardID:      1000,
			CardTitle:   "Card 1",
		},
	})

	body, err := json.Marshal(got[0].Boards[0].Columns[0].Cards[0])
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	want := `{"id":1000,"title":"Card 1","priority":null,"dueDate":null,"borderColor":null}`
	if string(body) != want {
		t.Fatalf("json = %s, want %s", body, want)
	}
}

func TestMapAssignedSubtasks(t *testing.T) {
	empty := mapAssignedSubtasks(nil)
	if empty == nil {
		t.Fatalf("empty result is nil, want empty slice")
	}
	if len(empty) != 0 {
		t.Fatalf("empty len = %d, want 0", len(empty))
	}

	got := mapAssignedSubtasks([]repository.AssignedSubtaskRow{
		{
			SubtaskID:     42,
			SubtaskTitle:  "Subtask",
			SubtaskStatus: "to_do",
			CardID:        555,
			CardTitle:     "Card",
			ColumnID:      100,
			ColumnTitle:   "Doing",
			BoardID:       10,
			BoardTitle:    "Board",
			ProjectID:     1,
			ProjectName:   "Project",
		},
	})

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ID != 42 || got[0].Title != "Subtask" || got[0].Status != "to_do" {
		t.Fatalf("subtask = %#v", got[0])
	}
	if got[0].Card.ID != 555 || got[0].Card.Title != "Card" {
		t.Fatalf("card ref = %#v", got[0].Card)
	}
	if got[0].Column.ID != 100 || got[0].Column.Title != "Doing" {
		t.Fatalf("column ref = %#v", got[0].Column)
	}
	if got[0].Board.ID != 10 || got[0].Board.Title != "Board" {
		t.Fatalf("board ref = %#v", got[0].Board)
	}
	if got[0].Project.ID != 1 || got[0].Project.Name != "Project" {
		t.Fatalf("project ref = %#v", got[0].Project)
	}
}
