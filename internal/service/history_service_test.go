package service

import "testing"

func TestUndoNested(t *testing.T) {
	for _, action := range []string{"card.created", "card.duplicated", "column.created", "board.created", "project.created"} {
		if !undoNested(action) {
			t.Fatalf("%s must check nested foreign history", action)
		}
	}
	if undoNested("card.updated.renamed") || undoNested("column.deleted") || undoNested("comment.created") {
		t.Fatal("non-create undo stays on the same entity")
	}
}
