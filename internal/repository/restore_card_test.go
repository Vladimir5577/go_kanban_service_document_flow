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

// TestRestoreCardRespectsBoardLimit — возврат из архива не пускает доску за
// предел.
//
// Возврат из архива увеличивает число активных карточек ровно так же, как
// создание, но раньше шёл обычной записью строки: ни подсчёта, ни блокировки.
// Доску с полным пределом можно было раздувать сколько угодно — заархивировать
// одну карточку, создать новую, вернуть архивную, повторить.
func TestRestoreCardRespectsBoardLimit(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	column := seedColumn(t, ctx, pool, board.BoardID, "колонка", 65536)
	for i := 0; i < testCardLimit; i++ {
		seedCard(t, ctx, pool, column, "активная", float64(65536*(i+1)))
	}
	archived := seedArchivedCard(t, ctx, pool, column, "в архиве", 1024)

	repo := &CardRepository{Db: pool}

	err := repo.RestoreCard(ctx, archived, board.BoardID, testCardLimit)
	if !errors.Is(err, apperr.New(apperr.CodeBoardCardLimitReached, "")) {
		t.Fatalf("возврат из архива при полном пределе должен отклоняться, а вернулось: %v", err)
	}
	if got := activeCardCount(t, ctx, pool, board.BoardID); got != testCardLimit {
		t.Errorf("активных карточек должно остаться %d, а их %d", testCardLimit, got)
	}

	// Освободили место — тот же возврат обязан пройти.
	if _, err := pool.Exec(ctx,
		`UPDATE kanban_card SET is_archived = TRUE WHERE id = (
		     SELECT id FROM kanban_card WHERE column_id = $1 AND is_archived = FALSE LIMIT 1
		 )`, column); err != nil {
		t.Fatalf("подготовка свободного места: %v", err)
	}
	if err := repo.RestoreCard(ctx, archived, board.BoardID, testCardLimit); err != nil {
		t.Fatalf("возврат из архива на свободное место: %v", err)
	}
	if got := activeCardCount(t, ctx, pool, board.BoardID); got != testCardLimit {
		t.Errorf("после возврата активных должно быть %d, а их %d", testCardLimit, got)
	}
}

// TestRestoreAndCreateCompeteForLastSlot — одновременные «создать» и «вернуть из
// архива» на одно свободное место.
//
// Обе операции увеличивают число активных карточек, поэтому обе обязаны брать
// блокировку доски. Если возврат из архива её не берёт, обе видят одинаковую
// картину, обе проходят проверку, и доска уезжает за предел.
func TestRestoreAndCreateCompeteForLastSlot(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	column := seedColumn(t, ctx, pool, board.BoardID, "колонка", 65536)
	for i := 1; i < testCardLimit; i++ {
		seedCard(t, ctx, pool, column, "активная", float64(65536*i))
	}
	archived := seedArchivedCard(t, ctx, pool, column, "в архиве", 1024)

	meet := newRendezvous(2, meetTimeout)
	repo := &CardRepository{Db: pool, testHookBeforeCardInsert: meet.arrive}

	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = repo.CreateCard(ctx, CreateCardInput{
			BoardID:        board.BoardID,
			ColumnID:       column,
			Card:           &model.Card{Title: "новая"},
			MaxActiveCards: testCardLimit,
		})
	}()
	go func() {
		defer wg.Done()
		errs[1] = repo.RestoreCard(ctx, archived, board.BoardID, testCardLimit)
	}()
	awaitAll(t, &wg, moveTimeout, "создание и возврат из архива")

	limitReached := apperr.New(apperr.CodeBoardCardLimitReached, "")
	succeeded := 0
	for i, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, limitReached):
		default:
			t.Fatalf("операция %d завершилась неожиданной ошибкой: %v", i+1, err)
		}
	}

	if succeeded != 1 {
		t.Fatalf("на одно свободное место должна пройти ровно одна операция, а прошло %d", succeeded)
	}
	if got := activeCardCount(t, ctx, pool, board.BoardID); got != testCardLimit {
		t.Errorf("активных карточек должно быть %d, а их %d", testCardLimit, got)
	}
}

func seedArchivedCard(t *testing.T, ctx context.Context, pool *pgxpool.Pool, columnID int64, title string, position float64) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO kanban_card (title, column_id, position, is_archived, archived_at)
		 VALUES ($1, $2, $3, TRUE, NOW()) RETURNING id`,
		title, columnID, position,
	).Scan(&id); err != nil {
		t.Fatalf("создание архивной карточки %s: %v", title, err)
	}
	return id
}

// TestRestoreCardGetsFreshPosition — карточка возвращается из архива на
// свободное место, а не на своё прежнее.
//
// Расчёт позиций смотрит только на активные карточки: пока карточка лежала в
// архиве, её место могла занять новая. Возврат со старым значением давал две
// активные карточки на одной позиции — порядок в колонке становился делом
// случая.
func TestRestoreCardGetsFreshPosition(t *testing.T) {
	ctx := context.Background()
	pool := newTestPool(t)

	board := seedBoard(t, ctx, pool)
	column := seedColumn(t, ctx, pool, board.BoardID, "колонка", 65536)

	// Место архивной карточки занято активной с той же позицией.
	archived := seedArchivedCard(t, ctx, pool, column, "в архиве", 65536)
	seedCard(t, ctx, pool, column, "занял место", 65536)

	repo := &CardRepository{Db: pool}
	if err := repo.RestoreCard(ctx, archived, board.BoardID, 0); err != nil {
		t.Fatalf("возврат из архива: %v", err)
	}

	assertPositionsDistinct(t, ctx, pool, column, "колонка")

	var position float64
	if err := pool.QueryRow(ctx, `SELECT position FROM kanban_card WHERE id = $1`, archived).Scan(&position); err != nil {
		t.Fatalf("чтение позиции вернувшейся карточки: %v", err)
	}
	if position <= 0 {
		t.Errorf("позиция вернувшейся карточки должна быть положительной, а получилась %v", position)
	}
}
