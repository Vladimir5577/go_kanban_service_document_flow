package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// meetTimeout — сколько участник ждёт остальных в точке встречи.
//
// Ожидание обязано быть конечным: при исправных блокировках второй участник до
// точки встречи не доходит вовсе — он стоит на блокировке колонки, — и первый
// должен пойти дальше сам, иначе тест заблокирует сам себя.
const meetTimeout = 2 * time.Second

// moveTimeout — предел на весь параллельный прогон.
const moveTimeout = 30 * time.Second

// TestMoveCardCounterMovesDoNotDeadlock — встречные переносы A→B и B→A.
//
// Что проверяем. Перенос идёт в одной транзакции и берёт блокировки двух
// колонок: исходной и целевой. Если брать их в порядке «сначала своя, потом
// чужая», две встречные транзакции возьмут ресурсы в противоположном порядке и
// встанут в петлю ожидания: PostgreSQL разорвёт её, сняв одну из транзакций с
// ошибкой 40P01. Для пользователя это выглядит как «карточка не перенеслась»
// без всякой причины.
//
// Защита — единый порядок по возрастанию id, независимо от того, откуда и куда
// едет карточка. Тогда вторая транзакция ждёт уже на первой блокировке, не
// успев тронуть ничего.
//
// Детерминизм. Точка встречи стоит между первой и второй блокировкой — только
// там видно, в каком порядке транзакции берут ресурсы. Пауз в тесте нет.
func TestMoveCardCounterMovesDoNotDeadlock(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)

	// Колонки создаются подряд, поэтому columnA.id < columnB.id: единый
	// порядок блокировок означает, что обе транзакции сначала берут A.
	columnA := seedColumn(t, ctx, pool, board.BoardID, "A", 65536)
	columnB := seedColumn(t, ctx, pool, board.BoardID, "B", 131072)

	// Соседи нужны, чтобы перенос попал в коллизию позиций и вызвал ребаланс:
	// именно ребаланс трогает строки, которые держит встречная транзакция.
	cardA := seedCard(t, ctx, pool, columnA, "карточка из A", 65536)
	seedCard(t, ctx, pool, columnA, "сосед в A", 131072)
	cardB := seedCard(t, ctx, pool, columnB, "карточка из B", 65536)
	seedCard(t, ctx, pool, columnB, "сосед в B", 131072)

	meet := newRendezvous(2, meetTimeout)
	repo := &CardRepository{Db: pool, testHookAfterFirstColumnLock: meet.arrive}

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		// A→B ровно на позицию соседа в B: коллизия, будет ребаланс B.
		_, errs[0] = repo.MoveCard(ctx, cardA, columnB, 131072, MoveCardOptions{})
	}()
	go func() {
		defer wg.Done()
		// B→A ровно на позицию соседа в A: коллизия, будет ребаланс A.
		_, errs[1] = repo.MoveCard(ctx, cardB, columnA, 131072, MoveCardOptions{})
	}()
	awaitAll(t, &wg, moveTimeout, "встречные переносы")

	for i, err := range errs {
		if err != nil {
			t.Fatalf("перенос %d завершился ошибкой: %v", i+1, err)
		}
	}

	// При исправном коде встреча не состоится: вторая транзакция ждёт на
	// первой же колонке. Если она вдруг состоялась, значит транзакции разошлись
	// по разным ресурсам — стоит знать об этом при разборе.
	if meet.happened() {
		t.Log("обе транзакции взяли первые блокировки одновременно: порядок захвата колонок не единый")
	}

	if got := cardColumn(t, ctx, pool, cardA); got != columnB {
		t.Errorf("карточка из A должна оказаться в колонке B (%d), а лежит в %d", columnB, got)
	}
	if got := cardColumn(t, ctx, pool, cardB); got != columnA {
		t.Errorf("карточка из B должна оказаться в колонке A (%d), а лежит в %d", columnA, got)
	}

	assertPositionsDistinct(t, ctx, pool, columnA, "A")
	assertPositionsDistinct(t, ctx, pool, columnB, "B")
}

// TestMoveCardParallelIntoSameColumnKeepsOrderUnique — два переноса из разных
// колонок в одну и ту же на одну и ту же позицию.
//
// Исходные колонки обязательно разные. Если брать обе карточки из одной
// колонки, транзакции выстроятся в очередь на блокировке источника, и тест
// останется зелёным, даже если блокировку колонки-приёмника убрать совсем, —
// то есть проверял бы он не то, ради чего написан.
//
// Что проверяем: колонка-приёмник блокируется, поэтому переносы в неё
// выстраиваются в очередь и каждый видит результат предыдущего. Без этой
// блокировки обе транзакции читают одинаковый снимок, обе решают, что коллизии
// нет, и записывают одинаковую позицию.
func TestMoveCardParallelIntoSameColumnKeepsOrderUnique(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	// Приёмник создаётся первым, чтобы его id был меньше: тогда при едином
	// порядке блокировок обе транзакции берут сначала именно его.
	target := seedColumn(t, ctx, pool, board.BoardID, "приёмник", 65536)
	sourceFirst := seedColumn(t, ctx, pool, board.BoardID, "источник 1", 131072)
	sourceSecond := seedColumn(t, ctx, pool, board.BoardID, "источник 2", 196608)

	first := seedCard(t, ctx, pool, sourceFirst, "первая", 65536)
	second := seedCard(t, ctx, pool, sourceSecond, "вторая", 65536)

	meet := newRendezvous(2, meetTimeout)
	repo := &CardRepository{Db: pool, testHookAfterFirstColumnLock: meet.arrive}

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = repo.MoveCard(ctx, first, target, 65536, MoveCardOptions{})
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = repo.MoveCard(ctx, second, target, 65536, MoveCardOptions{})
	}()
	awaitAll(t, &wg, moveTimeout, "параллельные переносы в одну колонку")

	for i, err := range errs {
		if err != nil {
			t.Fatalf("перенос %d завершился ошибкой: %v", i+1, err)
		}
	}

	if got := cardColumn(t, ctx, pool, first); got != target {
		t.Errorf("первая карточка должна быть в приёмнике (%d), а лежит в %d", target, got)
	}
	if got := cardColumn(t, ctx, pool, second); got != target {
		t.Errorf("вторая карточка должна быть в приёмнике (%d), а лежит в %d", target, got)
	}

	positions := columnPositions(t, ctx, pool, target)
	if len(positions) != 2 {
		t.Fatalf("в приёмнике ожидались две карточки, а их %d", len(positions))
	}
	assertPositionsDistinct(t, ctx, pool, target, "приёмник")
}

func assertPositionsDistinct(t *testing.T, ctx context.Context, pool *pgxpool.Pool, columnID int64, name string) {
	t.Helper()

	positions := columnPositions(t, ctx, pool, columnID)
	seen := make(map[float64]int, len(positions))
	for _, p := range positions {
		seen[p]++
		if seen[p] > 1 {
			t.Errorf("в колонке %s две карточки с одинаковой позицией %v: порядок доски перестал быть однозначным", name, p)
		}
	}
}
