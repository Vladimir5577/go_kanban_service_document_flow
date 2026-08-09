package repository

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/model"
)

// testCardLimit — предел карточек на доске в тестах. Реальный предел (300)
// проверяется тем же кодом, просто сеять три сотни карточек ради этого не
// нужно: важно поведение на границе, а не само число.
const testCardLimit = 3

// TestCreateCardConcurrentRespectsBoardLimitInOneColumn — два одновременных
// создания карточки, когда до предела остаётся ровно одно место.
//
// Что проверяем. Предел карточек на доске раньше проверялся до транзакции:
// обе попытки успевали прочитать «занято на одну меньше предела», обе
// проходили проверку и обе вставляли карточку. Доска уезжала за предел, и
// вернуть её обратно можно было только удалением карточек.
//
// Проверка и вставка теперь идут в одной транзакции под блокировкой доски,
// поэтому вторая попытка видит уже обновлённое число и получает отказ.
func TestCreateCardConcurrentRespectsBoardLimitInOneColumn(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	column := seedColumn(t, ctx, pool, board.BoardID, "колонка", 65536)
	for i := 1; i < testCardLimit; i++ {
		seedCard(t, ctx, pool, column, "уже лежит", float64(65536*i))
	}

	created, failed := createTwoCardsConcurrently(t, ctx, pool, board.BoardID, column, column)

	if created != 1 || failed != 1 {
		t.Fatalf("на одно свободное место должно пройти ровно одно создание, а прошло %d (отказов %d)", created, failed)
	}
	if got := activeCardCount(t, ctx, pool, board.BoardID); got != testCardLimit {
		t.Errorf("на доске должно остаться %d активных карточек, а их %d", testCardLimit, got)
	}
}

// TestCreateCardConcurrentRespectsBoardLimitAcrossColumns — то же самое, но
// карточки создаются в разных колонках одной доски.
//
// Отдельный тест, потому что защита здесь другая. Блокировки колонки для
// предела мало: колонки разные, транзакции не пересекаются и обе спокойно
// вставляют. Предел живёт на доске, значит и блокировка нужна на доске.
func TestCreateCardConcurrentRespectsBoardLimitAcrossColumns(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	first := seedColumn(t, ctx, pool, board.BoardID, "первая", 65536)
	second := seedColumn(t, ctx, pool, board.BoardID, "вторая", 131072)
	for i := 1; i < testCardLimit; i++ {
		seedCard(t, ctx, pool, first, "уже лежит", float64(65536*i))
	}

	created, failed := createTwoCardsConcurrently(t, ctx, pool, board.BoardID, first, second)

	if created != 1 || failed != 1 {
		t.Fatalf("на одно свободное место должно пройти ровно одно создание, а прошло %d (отказов %d)", created, failed)
	}
	if got := activeCardCount(t, ctx, pool, board.BoardID); got != testCardLimit {
		t.Errorf("на доске должно остаться %d активных карточек, а их %d", testCardLimit, got)
	}
}

// TestCreateCardConcurrentPositionsAreDistinct — две карточки, созданные
// одновременно в одной колонке, встают на разные места.
//
// Позиция новой карточки — половина позиции верхней. Пока расчёт жил в
// сервисе, оба запроса читали одну и ту же верхнюю карточку и получали
// одинаковое значение: порядок карточек в колонке после перезагрузки
// становился делом случая.
func TestCreateCardConcurrentPositionsAreDistinct(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	column := seedColumn(t, ctx, pool, board.BoardID, "колонка", 65536)
	seedCard(t, ctx, pool, column, "верхняя", 65536)

	created, failed := createTwoCardsConcurrently(t, ctx, pool, board.BoardID, column, column)
	if created != 2 || failed != 0 {
		t.Fatalf("предел не достигнут, обе карточки должны создаться, а создано %d (отказов %d)", created, failed)
	}

	assertPositionsDistinct(t, ctx, pool, column, "колонка")

	positions := columnPositions(t, ctx, pool, column)
	if len(positions) != 3 {
		t.Fatalf("в колонке ожидались три карточки, а их %d", len(positions))
	}
	for _, p := range positions {
		if p <= 0 {
			t.Errorf("позиция карточки должна оставаться положительной, а получилась %v", p)
		}
	}
}

// TestCreateCardRebalancesWhenPositionsRunOut — деление пополам не уводит
// позиции в ноль.
//
// Каждая карточка в начале колонки получает половину позиции верхней. Если
// добавлять их подряд, числа быстро становятся неразличимо малыми, а потом
// схлопываются в ноль — и порядок карточек перестаёт быть определённым.
// Дойдя до нижней границы, колонка раскладывается заново.
func TestCreateCardRebalancesWhenPositionsRunOut(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	column := seedColumn(t, ctx, pool, board.BoardID, "колонка", 65536)
	// Верхняя карточка стоит настолько близко к нулю, что делить дальше
	// некуда: следующая обязана получить позицию из пересчитанной колонки.
	seedCard(t, ctx, pool, column, "почти ноль", 0.5)

	repo := &CardRepository{Db: pool}
	card := &model.Card{Title: "новая"}
	if _, err := repo.CreateCard(ctx, CreateCardInput{
		BoardID:        board.BoardID,
		ColumnID:       column,
		Card:           card,
		MaxActiveCards: 0,
	}); err != nil {
		t.Fatalf("создание карточки: %v", err)
	}

	if card.Position < minCardPosition {
		t.Errorf("после пересчёта позиция должна быть не меньше %v, а получилась %v", minCardPosition, card.Position)
	}
	assertPositionsDistinct(t, ctx, pool, column, "колонка")
}

// createTwoCardsConcurrently запускает два создания карточки, синхронизированные
// в точке «предел проверен, позиция посчитана, вставки ещё не было».
//
// Возвращает, сколько попыток прошло и сколько отклонено по пределу. Любая
// другая ошибка валит тест: молча считать её отказом по пределу нельзя, иначе
// тест начнёт зеленеть на сломанном коде.
func createTwoCardsConcurrently(t *testing.T, ctx context.Context, pool *pgxpool.Pool, boardID, firstColumn, secondColumn int64) (created int, failed int) {
	t.Helper()

	meet := newRendezvous(2, meetTimeout)
	repo := &CardRepository{Db: pool, testHookBeforeCardInsert: meet.arrive}

	columns := [2]int64{firstColumn, secondColumn}
	errs := make([]error, 2)

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = repo.CreateCard(ctx, CreateCardInput{
				BoardID:        boardID,
				ColumnID:       columns[i],
				Card:           &model.Card{Title: "одновременная"},
				MaxActiveCards: testCardLimit,
			})
		}(i)
	}
	awaitAll(t, &wg, moveTimeout, "параллельные создания карточек")

	limitReached := apperr.New(apperr.CodeBoardCardLimitReached, "")
	for i, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, limitReached):
			failed++
		default:
			t.Fatalf("создание %d завершилось неожиданной ошибкой: %v", i+1, err)
		}
	}
	return created, failed
}

func activeCardCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, boardID int64) int {
	t.Helper()

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(c.id)
		   FROM kanban_card c
		   JOIN kanban_column col ON col.id = c.column_id
		  WHERE col.board_id = $1 AND c.is_archived = FALSE`,
		boardID,
	).Scan(&count); err != nil {
		t.Fatalf("подсчёт карточек доски: %v", err)
	}
	return count
}
