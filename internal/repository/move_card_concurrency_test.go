package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestMoveCardCounterMovesDoNotDeadlock — встречные переносы A→B и B→A.
//
// Что проверяем. Перенос карточки идёт в одной транзакции и трогает две
// колонки: карточку забирают из одной и кладут в другую, а при совпадении
// позиций ещё и пересчитывают порядок в колонке-приёмнике. Две такие
// транзакции, запущенные навстречу друг другу, берут одни и те же ресурсы в
// противоположном порядке — классическая петля ожидания. PostgreSQL разрывает
// её, снимая одну из транзакций с ошибкой 40P01, и для пользователя это
// выглядит как «карточка не перенеслась» без всякой причины.
//
// Защита — брать блокировки колонок в едином порядке по id, независимо от
// того, откуда и куда едет карточка. Тест существует, чтобы эту защиту нельзя
// было тихо потерять при рефакторинге: если убрать упорядочивание в MoveCard,
// тест падает на встречных переносах.
//
// Детерминизм. Одного «запустить две горутины» мало: без синхронизации они
// почти всегда расходятся по времени и петля не возникает. Поэтому обе
// транзакции встречаются в заранее известной точке — сразу после записи своей
// карточки и перед ребалансом чужой колонки. Пауз (sleep) в тесте нет.
func TestMoveCardCounterMovesDoNotDeadlock(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)

	// Колонки создаются подряд, поэтому columnA.id < columnB.id — важно для
	// проверки: единый порядок блокировок как раз и означает, что обе
	// транзакции сначала берут колонку с меньшим id.
	columnA := seedColumn(t, ctx, pool, board.BoardID, "A", 65536)
	columnB := seedColumn(t, ctx, pool, board.BoardID, "B", 131072)

	// В каждой колонке по две карточки. Соседи нужны, чтобы перенос попал в
	// коллизию позиций и вызвал ребаланс: именно ребаланс трогает строки,
	// которые в этот момент держит встречная транзакция.
	cardA := seedCard(t, ctx, pool, columnA, "карточка из A", 65536)
	neighborA := seedCard(t, ctx, pool, columnA, "сосед в A", 131072)
	cardB := seedCard(t, ctx, pool, columnB, "карточка из B", 65536)
	neighborB := seedCard(t, ctx, pool, columnB, "сосед в B", 131072)

	repo := &CardRepository{Db: pool}

	// Точка встречи. Таймаут — не «подождать подольше на всякий случай»: при
	// упорядоченных блокировках вторая транзакция до этой точки не доходит,
	// она ждёт на первой колонке. Значит первая обязана пойти дальше сама.
	meet := newRendezvous(2, 2*time.Second)
	testHookBeforeRebalance = meet.arrive
	t.Cleanup(func() { testHookBeforeRebalance = nil })

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		// A→B ровно на позицию соседа в B: коллизия, будет ребаланс B.
		_, errs[0] = repo.MoveCard(ctx, cardA, columnB, 131072)
	}()
	go func() {
		defer wg.Done()
		// B→A ровно на позицию соседа в A: коллизия, будет ребаланс A.
		_, errs[1] = repo.MoveCard(ctx, cardB, columnA, 131072)
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("встречные переносы не завершились за 30 секунд: похоже на взаимную блокировку, " +
			"которую не разобрал даже детектор PostgreSQL")
	}

	for i, err := range errs {
		if err != nil {
			t.Fatalf("перенос %d завершился ошибкой: %v", i+1, err)
		}
	}

	// Оба переноса обязаны доехать: ни один не должен быть отменён.
	if got := cardColumn(t, ctx, pool, cardA); got != columnB {
		t.Errorf("карточка из A должна оказаться в колонке B (%d), а лежит в %d", columnB, got)
	}
	if got := cardColumn(t, ctx, pool, cardB); got != columnA {
		t.Errorf("карточка из B должна оказаться в колонке A (%d), а лежит в %d", columnA, got)
	}

	// И порядок внутри колонок должен остаться однозначным: две карточки с
	// одинаковой позицией — это доска, которая после перезагрузки выглядит
	// по-разному у разных людей.
	assertPositionsDistinct(t, ctx, pool, columnA, "A")
	assertPositionsDistinct(t, ctx, pool, columnB, "B")

	_ = neighborA
	_ = neighborB
}

// TestMoveCardParallelIntoSameColumnKeepsOrderUnique — два переноса в одну и ту
// же колонку на одну и ту же позицию.
//
// Здесь взаимной блокировки нет, проверяется другое: колонка-приёмник
// блокируется, поэтому переносы выстраиваются в очередь и каждый видит
// результат предыдущего. Если блокировку колонки убрать, обе транзакции
// прочитают одинаковый снимок, обе решат, что коллизии нет, и запишут
// одинаковую позицию — порядок карточек станет зависеть от того, как база
// вернёт строки.
func TestMoveCardParallelIntoSameColumnKeepsOrderUnique(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	source := seedColumn(t, ctx, pool, board.BoardID, "источник", 65536)
	target := seedColumn(t, ctx, pool, board.BoardID, "приёмник", 131072)

	first := seedCard(t, ctx, pool, source, "первая", 65536)
	second := seedCard(t, ctx, pool, source, "вторая", 131072)
	seedCard(t, ctx, pool, target, "старожил", 65536)

	repo := &CardRepository{Db: pool}

	meet := newRendezvous(2, 2*time.Second)
	testHookBeforeRebalance = meet.arrive
	t.Cleanup(func() { testHookBeforeRebalance = nil })

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = repo.MoveCard(ctx, first, target, 65536)
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = repo.MoveCard(ctx, second, target, 65536)
	}()
	wg.Wait()

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
	if len(positions) != 3 {
		t.Fatalf("в приёмнике ожидались три карточки, а их %d", len(positions))
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
