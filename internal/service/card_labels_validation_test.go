package service

import (
	"context"
	"errors"
	"testing"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/model"
)

// labelRepoStub — минимальная заглушка LabelRepositoryInterface: в проверке
// участвует только GetLabels, остальное существует ради satisfying интерфейса.
type labelRepoStub struct {
	byBoard map[int64][]model.Label
	err     error
}

func (s *labelRepoStub) GetLabels(_ context.Context, boardID int64) ([]model.Label, error) {
	if s.err != nil {
		return nil, s.err
	}

	return s.byBoard[boardID], nil
}

func (s *labelRepoStub) GetLabel(context.Context, int64) (*model.Label, error) {
	return nil, errors.New("не используется в этих тестах")
}

func (s *labelRepoStub) CreateLabel(context.Context, int64, *model.Label) (*model.Label, error) {
	return nil, errors.New("не используется в этих тестах")
}

func (s *labelRepoStub) DeleteLabel(context.Context, int64) error {
	return errors.New("не используется в этих тестах")
}

func (s *labelRepoStub) ToggleLabel(context.Context, int64, int64) (bool, error) {
	return false, errors.New("не используется в этих тестах")
}

func TestValidateBoardLabelsAllowsOwnLabels(t *testing.T) {
	svc := &CardService{labelRepo: &labelRepoStub{byBoard: map[int64][]model.Label{
		7: {{ID: 1, BoardID: 7}, {ID: 2, BoardID: 7}},
	}}}

	if err := svc.validateBoardLabels(context.Background(), 7, []int64{1, 2}); err != nil {
		t.Fatalf("метки своей доски должны приниматься, получили: %v", err)
	}
}

func TestValidateBoardLabelsRejectsForeignLabel(t *testing.T) {
	svc := &CardService{labelRepo: &labelRepoStub{byBoard: map[int64][]model.Label{
		7: {{ID: 1, BoardID: 7}},
		9: {{ID: 42, BoardID: 9}},
	}}}

	err := svc.validateBoardLabels(context.Background(), 7, []int64{1, 42})
	if err == nil {
		t.Fatal("метка чужой доски не должна приниматься")
	}

	var appErr *apperr.Error
	if !errors.As(err, &appErr) || appErr.Code != apperr.CodeLabelNotFound {
		t.Errorf("ожидали код %q, получили %v", apperr.CodeLabelNotFound, err)
	}
}

func TestValidateBoardLabelsIgnoresEmptyList(t *testing.T) {
	// Пустой список не должен ходить в базу вовсе: стаб настроен на ошибку,
	// и если запрос всё-таки уйдёт, тест это увидит.
	svc := &CardService{labelRepo: &labelRepoStub{err: errors.New("не должно вызываться")}}

	if err := svc.validateBoardLabels(context.Background(), 7, nil); err != nil {
		t.Fatalf("пустой список меток — не ошибка, получили: %v", err)
	}
}
