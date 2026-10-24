package logger

import (
	"log/slog"
	"os"
)

// Setup: local/dev — текстовый лог уровня Debug, иначе — JSON уровня Info.
func Setup(env string) {
	var handler slog.Handler

	if env == "local" || env == "dev" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	} else {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
