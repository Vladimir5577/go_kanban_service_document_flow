package app

import (
	"net/http"
	"time"

	"go_kanban_service/internal/middleware"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

func setupRouter(h Handlers, authMw *middleware.AuthMiddleware) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestLogger())
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(15 * time.Second))

	// public API group
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "ok", "service": "kanban"}`))
	})

	// protected API group
	r.Group(func(r chi.Router) {
		r.Use(authMw.Handler)

		r.Get("/spa/api/kanban/me", h.User.LoginCheck())

		// PROJECTS (ProjectController)
		r.Route("/spa/api/kanban/projects", func(r chi.Router) {
			r.Get("/me", h.Project.GetMyProjects())
			r.Post("/", h.Project.CreateProject())
			r.Get("/{id}", h.Project.GetProject())
			r.Patch("/{id}", h.Project.UpdateProject())
			r.Patch("/{id}/move", h.Project.MoveProject())
			r.Delete("/{id}", h.Project.DeleteProject())

			// PROJECT MEMBERS
			r.Route("/{id}/members", func(r chi.Router) {
				r.Put("/", h.ProjectMember.ReplaceMembers())
				r.Patch("/{userId}", h.ProjectMember.UpdateMemberRole())
				r.Delete("/{userId}", h.ProjectMember.RemoveMember())
			})

			// BOARDS
			r.Route("/{id}/boards", func(r chi.Router) {
				r.Post("/", h.Board.CreateBoard())
				r.Get("/{boardId}", h.Board.GetBoard())
				r.Patch("/{boardId}", h.Board.UpdateBoard())
				r.Delete("/{boardId}", h.Board.DeleteBoard())
				r.Get("/{boardId}/archive", h.Board.GetBoardArchive())

				// COLUMNS
				r.Route("/{boardId}/columns", func(r chi.Router) {
					r.Post("/", h.Column.CreateColumn())
					r.Patch("/{columnId}", h.Column.UpdateColumn())
					r.Delete("/{columnId}", h.Column.DeleteColumn())
				})

				// LABELS
				r.Route("/{boardId}/labels", func(r chi.Router) {
					r.Get("/", h.Label.GetLabels())
					r.Post("/", h.Label.CreateLabel())
					r.Delete("/{labelId}", h.Label.DeleteLabel())
					r.Post("/cards/{cardId}/{labelId}", h.Label.ToggleLabel())
				})
			})
		})

		// PROJECT FOLDERS
		r.Route("/spa/api/kanban/project-folders", func(r chi.Router) {
			r.Get("/", h.ProjectFolder.GetProjectFolders())
		})

		// CARDS
		r.Route("/spa/api/kanban/cards", func(r chi.Router) {
			r.Post("/", h.Card.CreateCard())
			r.Get("/{id}", h.Card.GetCard())
			r.Patch("/{id}", h.Card.UpdateCard())
			r.Delete("/{id}", h.Card.DeleteCard())

			r.Put("/{id}/assignees", h.Card.UpdateAssignees())
			r.Post("/{id}/move", h.Card.MoveCard())
			r.Patch("/{id}/archive", h.Card.ArchiveCard())
			r.Patch("/{id}/complete", h.Card.CompleteCard())

			// SUBTASKS
			r.Route("/{cardId}/subtasks", func(r chi.Router) {
				r.Get("/", h.Subtask.GetSubtasks())
				r.Post("/", h.Subtask.CreateSubtask())
				r.Patch("/{id}", h.Subtask.UpdateSubtask())
				r.Delete("/{id}", h.Subtask.DeleteSubtask())
			})

			// COMMENTS
			r.Route("/{cardId}/comments", func(r chi.Router) {
				r.Get("/", h.Comment.GetComments())
				r.Post("/", h.Comment.CreateComment())
				r.Put("/{commentId}", h.Comment.UpdateComment())
				r.Delete("/{commentId}", h.Comment.DeleteComment())
			})

			// ATTACHMENTS
			r.Route("/{cardId}/attachments", func(r chi.Router) {
				r.Post("/", h.Attachment.UploadAttachment())
				r.Get("/{id}/download", h.Attachment.DownloadAttachment())
				r.Get("/{id}/preview", h.Attachment.PreviewAttachment())
				r.Delete("/{id}", h.Attachment.DeleteAttachment())
			})

			// ACTIVITIES
			r.Route("/{cardId}/activities", func(r chi.Router) {
				r.Get("/", h.Activity.GetActivities())
			})
		})
	})

	return r
}
