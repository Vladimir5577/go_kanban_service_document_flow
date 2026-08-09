package repository

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"


	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Общая обвязка интеграционных тестов репозитория.
//
// Проверки транзакций и блокировок нельзя сделать ни моками, ни `go test
// -race`: гонка происходит не в памяти процесса, а между транзакциями
// PostgreSQL. Нужна настоящая база — но своя, пустая и одноразовая, а не та,
// на которой работает стенд. Её поднимает docker-compose.test.yml.
//
// Адрес базы передаётся переменной KANBAN_TEST_DB_DSN. Если её нет, тесты
// пропускаются: обычный `go test ./...` должен проходить на машине, где нет
// Docker, иначе гейт превратится в «у меня локально красное, но это нормально».
const testDSNEnv = "KANBAN_TEST_DB_DSN"

// newTestPool отдаёт пул к тестовой базе со свежей схемой.
//
// Схема пересоздаётся перед каждым тестом целиком: это дешевле и надёжнее
// выборочной чистки таблиц — не нужно помнить порядок внешних ключей и нельзя
// случайно унаследовать строку от соседнего теста.
func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv(testDSNEnv)
	if dsn == "" {
		t.Skipf("нужен изолированный Postgres: docker compose -f docker-compose.test.yml up -d, "+
			"затем %s=postgres://test:test@127.0.0.1:55433/kanban_test?sslmode=disable", testDSNEnv)
	}

	assertThrowawayDatabase(t, dsn)

	// Подготовка схемы ограничена по времени: если база занята чужой
	// транзакцией, DROP SCHEMA будет ждать её сколько угодно, и прогон
	// замрёт без единого сообщения.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resetSchema(t, ctx, dsn)
	applyMigrations(t, ctx, dsn)

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("подключение к тестовой базе: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// assertThrowawayDatabase не даёт снести схему на чужой базе.
//
// Тесты начинают с DROP SCHEMA public CASCADE. Если в KANBAN_TEST_DB_DSN
// случайно окажется адрес стенда или рабочей базы, один запуск уничтожит её
// содержимое. Цена ошибки несопоставима с ценой проверки имени, поэтому
// разрешаем только базы, в имени которых есть test.
func assertThrowawayDatabase(t *testing.T, dsn string) {
	t.Helper()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("разбор %s: %v", testDSNEnv, err)
	}
	if !strings.Contains(strings.ToLower(cfg.Database), "test") {
		t.Fatalf("%s указывает на базу %q: тесты стирают схему целиком и работают только с одноразовой базой, "+
			"в имени которой есть test (см. docker-compose.test.yml)", testDSNEnv, cfg.Database)
	}
}

// simpleProtocolConn — соединение, через которое можно выполнить SQL-файл
// целиком. Расширенный протокол pgx отправляет по одному оператору за запрос и
// на многооператорной строке падает, а миграции — это именно такие файлы.
func simpleProtocolConn(t *testing.T, ctx context.Context, dsn string) *pgx.Conn {
	t.Helper()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("разбор %s: %v", testDSNEnv, err)
	}
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("подключение к тестовой базе: %v", err)
	}
	return conn
}

func resetSchema(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()

	conn := simpleProtocolConn(t, ctx, dsn)
	defer conn.Close(ctx)

	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("пересоздание схемы: %v", err)
	}
}

// applyMigrations накатывает миграции проекта.
//
// Goose как зависимость не добавляется: файлы миграций — обычный SQL, а
// секции разделены комментариями `-- +goose Up` / `-- +goose Down`. Берём
// первую и выполняем. Так схема в тестах гарантированно та же, что на стенде,
// без лишнего инструмента в go.mod.
func applyMigrations(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil {
		t.Fatalf("поиск миграций: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("миграции не найдены: ожидался каталог migrations в корне репозитория")
	}
	sort.Strings(files)

	conn := simpleProtocolConn(t, ctx, dsn)
	defer conn.Close(ctx)

	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("чтение %s: %v", file, err)
		}

		up := upSection(string(raw))
		if strings.TrimSpace(up) == "" {
			t.Fatalf("в %s не найдена секция +goose Up", file)
		}
		if _, err := conn.Exec(ctx, up); err != nil {
			t.Fatalf("применение %s: %v", filepath.Base(file), err)
		}
	}
}

func upSection(migration string) string {
	const upMarker = "-- +goose Up"
	const downMarker = "-- +goose Down"

	start := strings.Index(migration, upMarker)
	if start < 0 {
		return ""
	}
	body := migration[start+len(upMarker):]

	if end := strings.Index(body, downMarker); end >= 0 {
		body = body[:end]
	}
	// Служебные пометки goose внутри секции на выполнение не влияют, но и
	// смысла в них для psql нет — убираем, чтобы не мусорить в логе ошибок.
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "-- +goose") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// testBoard — минимальный набор идентификаторов, который нужен тестам карточек.
type testBoard struct {
	ProjectID int64
	BoardID   int64
}

func seedBoard(t *testing.T, ctx context.Context, pool *pgxpool.Pool) testBoard {
	t.Helper()

	var b testBoard
	if err := pool.QueryRow(ctx,
		`INSERT INTO kanban_project (name, owner_id) VALUES ('тестовый проект', 1) RETURNING id`,
	).Scan(&b.ProjectID); err != nil {
		t.Fatalf("создание проекта: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO kanban_board (title, kanban_project_id, created_by_id)
		 VALUES ('тестовая доска', $1, 1) RETURNING id`,
		b.ProjectID,
	).Scan(&b.BoardID); err != nil {
		t.Fatalf("создание доски: %v", err)
	}
	return b
}

func seedColumn(t *testing.T, ctx context.Context, pool *pgxpool.Pool, boardID int64, title string, position float64) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO kanban_column (title, board_id, position) VALUES ($1, $2, $3) RETURNING id`,
		title, boardID, position,
	).Scan(&id); err != nil {
		t.Fatalf("создание колонки %s: %v", title, err)
	}
	return id
}

func seedCard(t *testing.T, ctx context.Context, pool *pgxpool.Pool, columnID int64, title string, position float64) int64 {
	t.Helper()

	var id int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO kanban_card (title, column_id, position) VALUES ($1, $2, $3) RETURNING id`,
		title, columnID, position,
	).Scan(&id); err != nil {
		t.Fatalf("создание карточки %s: %v", title, err)
	}
	return id
}

func cardColumn(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cardID int64) int64 {
	t.Helper()

	var columnID int64
	if err := pool.QueryRow(ctx, `SELECT column_id FROM kanban_card WHERE id = $1`, cardID).Scan(&columnID); err != nil {
		t.Fatalf("чтение колонки карточки %d: %v", cardID, err)
	}
	return columnID
}

// columnPositions отдаёт позиции карточек колонки в порядке возрастания.
func columnPositions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, columnID int64) []float64 {
	t.Helper()

	rows, err := pool.Query(ctx,
		`SELECT position FROM kanban_card WHERE column_id = $1 AND is_archived = FALSE ORDER BY position`,
		columnID,
	)
	if err != nil {
		t.Fatalf("чтение позиций колонки %d: %v", columnID, err)
	}
	defer rows.Close()

	var positions []float64
	for rows.Next() {
		var p float64
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("чтение позиции: %v", err)
		}
		positions = append(positions, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("чтение позиций колонки %d: %v", columnID, err)
	}
	return positions
}

// rendezvous — встреча двух горутин в заранее известной точке.
//
// Нужна вместо sleep: пауза «на всякий случай» либо не воспроизводит гонку,
// либо замедляет прогон, и в обоих случаях результат теста зависит от
// загрузки машины. Здесь же участники ждут именно друг друга.
//
// Ожидание ограничено таймаутом, и это не страховка, а часть замысла: когда
// блокировки берутся в едином порядке, второй участник до точки встречи не
// доходит — он ждёт на блокировке колонки. Первый обязан в этом случае пойти
// дальше и завершить транзакцию, иначе тест сам себя заблокирует.
type rendezvous struct {
	mu      sync.Mutex
	arrived int
	want    int
	ready   chan struct{}
	timeout time.Duration
}

func newRendezvous(want int, timeout time.Duration) *rendezvous {
	return &rendezvous{want: want, ready: make(chan struct{}), timeout: timeout}
}

func (r *rendezvous) arrive() {
	r.mu.Lock()
	r.arrived++
	if r.arrived == r.want {
		close(r.ready)
	}
	r.mu.Unlock()

	select {
	case <-r.ready:
	case <-time.After(r.timeout):
	}
}

// happened говорит, дошли ли до точки встречи все участники.
//
// Это не проверка успеха, а способ понять, что именно проверил тест: встреча
// состоялась — значит чередование было настоящим; не состоялась — значит
// второй участник ждал на блокировке, и это тоже осмысленный результат.
// Без такого различения зелёный тест, в котором хук перестал вызываться
// вовсе, выглядел бы точно так же, как честно пройденный.
func (r *rendezvous) happened() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.arrived >= r.want
}

// awaitAll ждёт завершения горутин теста с ограничением по времени.
//
// Голый wg.Wait() при регрессии в блокировках оставляет прогон висеть до
// общего таймаута go test — без сообщения о том, что именно застряло.
func awaitAll(t *testing.T, wg *sync.WaitGroup, timeout time.Duration, what string) {
	t.Helper()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s не завершились за %s: похоже на взаимную блокировку, которую не разобрал даже детектор PostgreSQL", what, timeout)
	}
}
