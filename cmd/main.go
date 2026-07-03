package main

import (
	"log/slog"
	"os"

	"go_kanban_service/internal/app"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/logger"
	"go_kanban_service/internal/validator"
)

func main() {
	cfg := config.Load()

	logger.Setup(cfg.Env)
	validator.Init()

	db, err := config.ConnectDB(cfg)
	if err != nil {
		slog.Error("Can't connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	application, err := app.NewApp(cfg, db)
	if err != nil {
		slog.Error("Can't initialize application", "error", err)
		os.Exit(1)
	}

	slog.Info("Канбан-микросервис запускается", "port", cfg.Port)
	if err := application.Run(); err != nil {
		slog.Error("Ошибка старта HTTP-сервера", "error", err)
		os.Exit(1)
	}
}
