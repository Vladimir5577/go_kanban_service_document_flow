package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go_kanban_service/internal/config"
	"go_kanban_service/internal/handler"
	"go_kanban_service/internal/middleware"
	"go_kanban_service/internal/repository"
	"go_kanban_service/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type App struct {
	router *chi.Mux
	cfg    *config.Config
}

type Handlers struct {
	User          *handler.UserHandler
	Project       *handler.ProjectHandler
	Activity      *handler.ActivityHandler
	Attachment    *handler.AttachmentHandler
	Board         *handler.BoardHandler
	Card          *handler.CardHandler
	Column        *handler.ColumnHandler
	Comment       *handler.CommentHandler
	Label         *handler.LabelHandler
	ProjectFolder *handler.ProjectFolderHandler
	ProjectMember *handler.ProjectMemberHandler
	Subtask       *handler.SubtaskHandler
}

func NewApp(cfg *config.Config, db *pgxpool.Pool) (*App, error) {
	authMw, err := middleware.NewAuthMiddleware(cfg.JWTPublicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init auth middleware: %w", err)
	}

	minioSvc, err := service.NewMinioService(
		cfg.MinioEndpoint,
		cfg.MinioAccessKeyID,
		cfg.MinioSecretAccessKey,
		cfg.MinioUseSSL,
		cfg.MinioBucket,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to init minio service: %w", err)
	}

	projectRepo := repository.NewProjectRepository(db)
	projectMemberRepo := repository.NewProjectMemberRepository(db)

	permSvc := service.NewPermissionService(db, projectRepo, projectMemberRepo)

	userRepo := repository.NewUserRepository(db)
	userSvc := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userSvc)

	activityRepo := repository.NewActivityRepository(db)
	activitySvc := service.NewActivityService(activityRepo, permSvc)
	activityHandler := handler.NewActivityHandler(activitySvc)

	attachmentRepo := repository.NewAttachmentRepository(db)
	subtaskRepo := repository.NewSubtaskRepository(db)
	commentRepo := repository.NewCommentRepository(db)
	labelRepo := repository.NewLabelRepository(db)

	attachmentSvc := service.NewAttachmentService(attachmentRepo, permSvc, activityRepo)
	attachmentHandler := handler.NewAttachmentHandler(attachmentSvc, minioSvc, cfg)

	boardRepo := repository.NewBoardRepository(db)
	columnRepo := repository.NewColumnRepository(db)

	cardRepo := repository.NewCardRepository(db)
	cardSvc := service.NewCardService(cardRepo, permSvc, minioSvc, subtaskRepo, commentRepo, attachmentRepo, labelRepo, userRepo, activityRepo, columnRepo, projectRepo, projectMemberRepo, cfg)
	cardHandler := handler.NewCardHandler(cardSvc)

	columnSvc := service.NewColumnService(columnRepo, permSvc, boardRepo)
	columnHandler := handler.NewColumnHandler(columnSvc)

	commentSvc := service.NewCommentService(commentRepo, permSvc, userRepo)
	commentHandler := handler.NewCommentHandler(commentSvc)

	labelSvc := service.NewLabelService(labelRepo, permSvc, activityRepo, boardRepo, cardRepo, columnRepo)
	labelHandler := handler.NewLabelHandler(labelSvc)

	projectFolderRepo := repository.NewProjectFolderRepository(db)
	projectFolderSvc := service.NewProjectFolderService(projectFolderRepo, permSvc)
	projectFolderHandler := handler.NewProjectFolderHandler(projectFolderSvc)

	projectSvc := service.NewProjectService(projectRepo, boardRepo, projectMemberRepo, userRepo, permSvc)
	projectHandler := handler.NewProjectHandler(projectSvc)

	projectMemberSvc := service.NewProjectMemberService(projectMemberRepo, userRepo, permSvc)
	projectMemberHandler := handler.NewProjectMemberHandler(projectMemberSvc, projectSvc)

	subtaskSvc := service.NewSubtaskService(subtaskRepo, permSvc, activityRepo, userRepo, projectRepo, projectMemberRepo)
	subtaskHandler := handler.NewSubtaskHandler(subtaskSvc)

	boardSvc := service.NewBoardService(boardRepo, columnRepo, cardRepo, labelRepo, userRepo, subtaskRepo, commentRepo, attachmentRepo, permSvc)
	boardHandler := handler.NewBoardHandler(boardSvc)

	h := Handlers{
		User:          userHandler,
		Project:       projectHandler,
		Activity:      activityHandler,
		Attachment:    attachmentHandler,
		Board:         boardHandler,
		Card:          cardHandler,
		Column:        columnHandler,
		Comment:       commentHandler,
		Label:         labelHandler,
		ProjectFolder: projectFolderHandler,
		ProjectMember: projectMemberHandler,
		Subtask:       subtaskHandler,
	}

	r := setupRouter(h, authMw)

	return &App{
		router: r,
		cfg:    cfg,
	}, nil
}

func (a *App) Run() error {
	addr := fmt.Sprintf(":%s", a.cfg.Port)

	srv := &http.Server{
		Addr:         addr,
		Handler:      a.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 20 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Запускаем сервер в горутине
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Ошибка старта HTTP-сервера", "error", err)
			os.Exit(1)
		}
	}()

	// Ожидаем сигналы ОС для graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Получен сигнал завершения, начинаем graceful shutdown...")

	// Даем 5 секунд на завершение текущих запросов
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("ошибка при остановке сервера: %w", err)
	}

	slog.Info("HTTP-сервер успешно остановлен")
	return nil
}
