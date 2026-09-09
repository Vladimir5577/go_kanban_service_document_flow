package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"go_kanban_service/internal/model"
)

// Контракт с фронтом: поле с позициями присутствует в ответе всегда —
// null, если ребалансировки не было, и массив, если была.
// Поля самой сущности при этом остаются в корне ответа, как до правки.

func TestMoveCardResponseRebalancedCards(t *testing.T) {
	move := &model.CardMove{ID: 7, ToColumnID: 2, Position: 3.5}

	body, err := json.Marshal(MapMoveCardResponse(move))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"rebalancedCards":null`) {
		t.Fatalf("без ребалансировки ожидали null, получили: %s", body)
	}
	if !strings.Contains(string(body), `"id":7`) ||
		!strings.Contains(string(body), `"columnId":2`) ||
		!strings.Contains(string(body), `"position":3.5`) {
		t.Fatalf("в ответе должны быть id, columnId и position: %s", body)
	}

	move.Position = 65536
	move.Rebalanced = []model.CardPosition{
		{ID: 7, Position: 65536},
		{ID: 9, Position: 131072},
	}
	body, err = json.Marshal(MapMoveCardResponse(move))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"rebalancedCards":[{"id":7,"position":65536},{"id":9,"position":131072}]`) {
		t.Fatalf("после ребалансировки ожидали массив позиций, получили: %s", body)
	}
}

func TestUpdateColumnResponseRebalancedColumns(t *testing.T) {
	column := MapColumnResponse(&model.Column{ID: 4, Title: "column", Position: 3.5, BoardID: 1})

	body, err := json.Marshal(&UpdateColumnResponse{
		ColumnResponse:    column,
		RebalancedColumns: MapColumnPositions(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"rebalancedColumns":null`) {
		t.Fatalf("без ребалансировки ожидали null, получили: %s", body)
	}
	if !strings.Contains(string(body), `"id":4`) || !strings.Contains(string(body), `"position":3.5`) {
		t.Fatalf("поля колонки должны лежать в корне ответа: %s", body)
	}

	body, err = json.Marshal(&UpdateColumnResponse{
		ColumnResponse: column,
		RebalancedColumns: MapColumnPositions([]model.Column{
			{ID: 4, Position: 65536},
			{ID: 6, Position: 131072},
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"rebalancedColumns":[{"id":4,"position":65536},{"id":6,"position":131072}]`) {
		t.Fatalf("после ребалансировки ожидали массив позиций, получили: %s", body)
	}
}
