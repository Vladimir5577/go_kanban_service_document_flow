package service

import (
	"context"
	"testing"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

// Фейки встраивают интерфейс репозитория: любой не переопределённый метод —
// nil-вызов и паника. Так тест ловит и лишний GetCard, и возврат GetColumn.
type restoredCardRepo struct {
	repository.CardRepositoryInterface
	gets int
}

func (r *restoredCardRepo) GetCard(_ context.Context, id int64) (*model.Card, error) {
	r.gets++
	return &model.Card{ID: id, Title: "t", ColumnID: 5, AssigneeIDs: []int64{9}, LabelIDs: []int64{2}}, nil
}

func (r *restoredCardRepo) GetChildCountsByParentIDs(_ context.Context, ids []int64) (map[int64]model.ChecklistCount, error) {
	return map[int64]model.ChecklistCount{ids[0]: {Total: 3, Done: 1}}, nil
}

type restoredLabelRepo struct {
	repository.LabelRepositoryInterface
}

func (restoredLabelRepo) GetLabels(_ context.Context, boardID int64) ([]model.Label, error) {
	if boardID != 42 {
		return nil, nil
	}
	return []model.Label{{ID: 1, Name: "чужая"}, {ID: 2, Name: "своя"}}, nil
}

type restoredUserRepo struct {
	repository.UserRepositoryInterface
}

func (restoredUserRepo) GetUsersByIDs(_ context.Context, ids []int64) ([]model.User, error) {
	return []model.User{{ID: ids[0], Firstname: "Иван"}}, nil
}

type restoredCommentRepo struct {
	repository.CommentRepositoryInterface
}

func (restoredCommentRepo) GetCountsByCardIDs(_ context.Context, ids []int64) (map[int64]int, error) {
	return map[int64]int{ids[0]: 2}, nil
}

type restoredAttachmentRepo struct {
	repository.AttachmentRepositoryInterface
}

func (restoredAttachmentRepo) GetChatCountsByCardIDs(_ context.Context, ids []int64) (map[int64]int, error) {
	return map[int64]int{ids[0]: 1}, nil
}

func TestBuildRestoredCardReadsCardOnce(t *testing.T) {
	cards := &restoredCardRepo{}
	p := NewKanbanRealtimePublisher("", "", cards, nil, restoredCommentRepo{}, restoredAttachmentRepo{}, restoredLabelRepo{}, restoredUserRepo{}, nil)

	got, err := p.BuildRestoredCard(context.Background(), 7, 42)
	if err != nil {
		t.Fatal(err)
	}
	if cards.gets != 1 {
		t.Fatalf("GetCard calls = %d, want 1", cards.gets)
	}
	if got["boardId"] != int64(42) || got["columnId"] != int64(5) {
		t.Fatalf("boardId/columnId = %v/%v, want 42/5", got["boardId"], got["columnId"])
	}
	if labels := got["labels"].([]map[string]any); len(labels) != 1 || labels[0]["id"] != int64(2) {
		t.Fatalf("labels = %v, want only label 2", labels)
	}
	if assignees := got["assignees"].([]map[string]any); len(assignees) != 1 || assignees[0]["id"] != int64(9) {
		t.Fatalf("assignees = %v, want user 9", assignees)
	}
	if got["checklistTotal"] != 3 || got["checklistDone"] != 1 || got["commentsCount"] != 3 {
		t.Fatalf("counters = %v/%v/%v, want 3/1/3", got["checklistTotal"], got["checklistDone"], got["commentsCount"])
	}
}
