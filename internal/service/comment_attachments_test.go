package service

import (
	"testing"

	"go_kanban_service/internal/model"
)

func TestAttachFilesToComments(t *testing.T) {
	comments := []model.Comment{{ID: 1}, {ID: 2}, {ID: 3}}
	files := []model.CommentAttachment{
		{ID: 11, CommentID: 1},
		{ID: 12, CommentID: 2},
		{ID: 13, CommentID: 1},
		{ID: 14, CommentID: 99}, // файл удалённого комментария — никуда не ложится
	}

	AttachFilesToComments(comments, files)

	if len(comments[0].Attachments) != 2 || comments[0].Attachments[0].ID != 11 || comments[0].Attachments[1].ID != 13 {
		t.Fatalf("comment 1 files = %v, want [11 13]", comments[0].Attachments)
	}
	if len(comments[1].Attachments) != 1 || comments[1].Attachments[0].ID != 12 {
		t.Fatalf("comment 2 files = %v, want [12]", comments[1].Attachments)
	}
	if comments[2].Attachments == nil || len(comments[2].Attachments) != 0 {
		t.Fatalf("comment 3 files = %#v, want empty non-nil", comments[2].Attachments)
	}
}
