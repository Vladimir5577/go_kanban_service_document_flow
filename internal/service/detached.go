package service

import (
	"context"
	"log/slog"
	"time"
)

// Таймаут фоновой рассылки уведомлений: несколько запросов в базу плюс отправка
// события. Больше, чем у Mercure, где всего одна HTTP-отправка.
const notifyTimeout = 10 * time.Second

// runDetached выполняет работу в фоне, чтобы HTTP-ответ её не ждал.
//
// Обычный `go fn(ctx)` здесь не годится: контекст запроса отменяется сразу после
// того, как хэндлер отдал ответ, и работа умрёт, не начавшись. context.WithoutCancel
// сохраняет значения ctx (пользователь, request id), но снимает отмену запроса,
// а срок жизни фоновой работе задаёт собственный таймаут.
//
// ponytail: горутина на вызов, без лимита. Для нагрузки доски норм; пул воркеров —
// если упрёшься.
func runDetached(ctx context.Context, timeout time.Duration, what string, fn func(context.Context) error) {
	if fn == nil {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(detached, timeout)
		defer cancel()
		if err := fn(ctx); err != nil {
			slog.WarnContext(ctx, what, "error", err)
		}
	}()
}
