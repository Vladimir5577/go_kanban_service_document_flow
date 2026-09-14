package repository

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"go_kanban_service/internal/apperr"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

func applyHistoryUndo(ctx context.Context, q *dbgen.Queries, e model.HistoryUndoEntry) error {
	switch e.Action {
	case "card.created", "card.duplicated":
		return q.DeleteCard(ctx, e.EntityID)
	case "card.deleted":
		return q.RestoreCard(ctx, e.EntityID)
	case "card.moved":
		colID, pos, ok := parseUndoPlacement(e.Before)
		if !ok {
			return undoImpossible()
		}
		card, err := q.GetCard(ctx, e.EntityID)
		if err != nil {
			return err
		}
		return updateCardRow(ctx, q, card, func(p *dbgen.UpdateCardParams) {
			p.ColumnID = colID
			p.Position = pos
		})
	case "card.updated.renamed":
		if e.Before == "" {
			return undoImpossible()
		}
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) { p.Title = e.Before })
	case "card.updated.description":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.Description = textFromAny(e.Before)
			if e.Before == "" {
				p.Description = pgtype.Text{}
			}
		})
	case "card.updated.due_date":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.DueDate = parseUndoDue(e.Before)
		})
	case "card.updated.priority":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.Priority = textFromAny(e.Before)
			if e.Before == "" {
				p.Priority = pgtype.Text{}
			}
		})
	case "card.updated.color":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.BorderColor = textFromAny(e.Before)
			if e.Before == "" {
				p.BorderColor = pgtype.Text{}
			}
		})
	case "card.archived":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.IsArchived = false
			p.ArchivedAt = pgtype.Timestamptz{}
			p.ArchivedByID = pgtype.Int8{}
		})
	case "card.restored":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.IsArchived = true
		})
	case "card.completed":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.CompletedAt = pgtype.Timestamptz{}
			p.CompletedByID = pgtype.Int8{}
		})
	case "card.reopened":
		return patchCard(ctx, q, e.EntityID, func(p *dbgen.UpdateCardParams) {
			p.CompletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		})
	case "card.assignee_added", "card.assignee_removed", "card.assignees":
		return restoreCardAssignee(ctx, q, e.EntityID, e.Before)
	case "column.created":
		return q.DeleteColumn(ctx, e.EntityID)
	case "column.deleted":
		return q.RestoreColumn(ctx, e.EntityID)
	case "column.updated.renamed":
		if e.Before == "" {
			return undoImpossible()
		}
		return patchColumn(ctx, q, e.EntityID, func(title, color *string, pos *float64) { *title = e.Before })
	case "column.updated.color":
		if e.Before == "" {
			return undoImpossible()
		}
		return patchColumn(ctx, q, e.EntityID, func(title, color *string, pos *float64) { *color = e.Before })
	case "column.updated.moved":
		pos, ok := parseUndoPos(e.Before)
		if !ok {
			return undoImpossible()
		}
		return patchColumn(ctx, q, e.EntityID, func(title, color *string, p *float64) { *p = pos })
	case "board.created":
		return q.DeleteBoard(ctx, e.EntityID)
	case "board.deleted":
		return q.RestoreBoard(ctx, e.EntityID)
	case "board.updated.renamed":
		if e.Before == "" {
			return undoImpossible()
		}
		return patchBoard(ctx, q, e.EntityID, e.Before, nil, false, nil)
	case "board.updated.moved":
		pos, ok := parseUndoPos(e.Before)
		if !ok {
			return undoImpossible()
		}
		return patchBoard(ctx, q, e.EntityID, "", &pos, false, nil)
	case "board.updated.done_column":
		var done *int64
		if e.Before != "" {
			id, err := strconv.ParseInt(e.Before, 10, 64)
			if err != nil {
				return undoImpossible()
			}
			done = &id
		}
		return patchBoard(ctx, q, e.EntityID, "", nil, true, done)
	case "project.created":
		return q.DeleteProject(ctx, e.EntityID)
	case "project.deleted":
		return q.RestoreProject(ctx, e.EntityID)
	case "project.updated.renamed":
		if e.Before == "" {
			return undoImpossible()
		}
		return patchProject(ctx, q, e.EntityID, &e.Before, nil, false)
	case "project.updated.description":
		return patchProject(ctx, q, e.EntityID, nil, &e.Before, true)
	case "label.created":
		return q.DeleteLabel(ctx, e.EntityID)
	case "label.deleted":
		return q.RestoreLabel(ctx, e.EntityID)
	case "label.added":
		if e.CardID == 0 {
			return undoImpossible()
		}
		return q.RemoveCardLabel(ctx, dbgen.RemoveCardLabelParams{KanbanCardID: e.CardID, KanbanLabelID: e.EntityID})
	case "label.removed":
		if e.CardID == 0 {
			return undoImpossible()
		}
		return q.AddCardLabel(ctx, dbgen.AddCardLabelParams{KanbanCardID: e.CardID, KanbanLabelID: e.EntityID})
	case "comment.created":
		return q.DeleteComment(ctx, e.EntityID)
	case "comment.deleted":
		return q.RestoreComment(ctx, e.EntityID)
	case "comment.updated":
		_, err := q.UpdateComment(ctx, dbgen.UpdateCommentParams{Body: e.Before, ID: e.EntityID})
		return err
	case "subtask.created":
		return q.DeleteSubtask(ctx, e.EntityID)
	case "subtask.deleted":
		return q.RestoreSubtask(ctx, e.EntityID)
	case "subtask.renamed":
		if e.Before == "" {
			return undoImpossible()
		}
		return patchSubtask(ctx, q, e.EntityID, &e.Before, nil, nil, false, nil)
	case "subtask.completed":
		status := "todo"
		return patchSubtask(ctx, q, e.EntityID, nil, &status, nil, false, nil)
	case "subtask.reopened":
		status := "done"
		return patchSubtask(ctx, q, e.EntityID, nil, &status, nil, false, nil)
	case "subtask.moved":
		pos, ok := parseUndoPos(e.Before)
		if !ok {
			return undoImpossible()
		}
		return patchSubtask(ctx, q, e.EntityID, nil, nil, &pos, false, nil)
	case "subtask.assigned", "subtask.unassigned":
		if e.Before == "" {
			return patchSubtask(ctx, q, e.EntityID, nil, nil, nil, true, nil)
		}
		uid, ok := parseUndoID(e.Before)
		if !ok {
			return undoImpossible()
		}
		return patchSubtask(ctx, q, e.EntityID, nil, nil, nil, false, &uid)
	case "attachment.created":
		return q.DeleteAttachment(ctx, e.EntityID)
	case "attachment.deleted":
		return q.RestoreAttachment(ctx, e.EntityID)
	default:
		return undoImpossible()
	}
}

func undoImpossible() error {
	return apperr.New(apperr.CodeUndoImpossible, "Отмена невозможна")
}

func patchCard(ctx context.Context, q *dbgen.Queries, id int64, mutate func(*dbgen.UpdateCardParams)) error {
	card, err := q.GetCard(ctx, id)
	if err != nil {
		return err
	}
	return updateCardRow(ctx, q, card, mutate)
}

func patchColumn(ctx context.Context, q *dbgen.Queries, id int64, mutate func(title, color *string, pos *float64)) error {
	col, err := q.GetColumn(ctx, id)
	if err != nil {
		return err
	}
	title, color, pos := col.Title, col.HeaderColor, col.Position
	mutate(&title, &color, &pos)
	_, err = q.UpdateColumn(ctx, dbgen.UpdateColumnParams{
		Title: title, HeaderColor: color, Position: pos, ID: col.ID,
	})
	return err
}

func patchBoard(ctx context.Context, q *dbgen.Queries, id int64, title string, pos *float64, setDone bool, done *int64) error {
	board, err := q.GetBoard(ctx, id)
	if err != nil {
		return err
	}
	nextTitle := board.Title
	if title != "" {
		nextTitle = title
	}
	nextPos := board.Position
	if pos != nil {
		nextPos = *pos
	}
	if _, err := q.UpdateBoard(ctx, dbgen.UpdateBoardParams{Title: nextTitle, Position: nextPos, ID: board.ID}); err != nil {
		return err
	}
	if !setDone {
		return nil
	}
	arg := pgtype.Int8{}
	if done != nil {
		arg = pgtype.Int8{Int64: *done, Valid: true}
	}
	return q.SetDoneColumnID(ctx, dbgen.SetDoneColumnIDParams{DoneColumnID: arg, ID: board.ID})
}

func patchProject(ctx context.Context, q *dbgen.Queries, id int64, name *string, desc *string, setDesc bool) error {
	p, err := q.GetProject(ctx, id)
	if err != nil {
		return err
	}
	nextName := p.Name
	if name != nil {
		nextName = *name
	}
	nextDesc := p.Description
	if setDesc {
		if desc == nil || *desc == "" {
			nextDesc = pgtype.Text{}
		} else {
			nextDesc = pgtype.Text{String: *desc, Valid: true}
		}
	}
	_, err = q.UpdateProject(ctx, dbgen.UpdateProjectParams{Name: nextName, Description: nextDesc, ID: p.ID})
	return err
}

func patchSubtask(ctx context.Context, q *dbgen.Queries, id int64, title, status *string, pos *float64, clearUser bool, userID *int64) error {
	st, err := q.GetSubtask(ctx, id)
	if err != nil {
		return err
	}
	nextTitle, nextStatus, nextPos, nextUser := st.Title, st.Status, st.Position, st.UserID
	if title != nil {
		nextTitle = *title
	}
	if status != nil {
		nextStatus = *status
	}
	if pos != nil {
		nextPos = *pos
	}
	if clearUser {
		nextUser = pgtype.Int8{}
	} else if userID != nil {
		nextUser = pgtype.Int8{Int64: *userID, Valid: true}
	}
	_, err = q.UpdateSubtask(ctx, dbgen.UpdateSubtaskParams{
		Title: nextTitle, Status: nextStatus, Position: nextPos, UserID: nextUser, ID: st.ID,
	})
	return err
}

func updateCardRow(ctx context.Context, q *dbgen.Queries, card dbgen.KanbanCard, mutate func(*dbgen.UpdateCardParams)) error {
	params := dbgen.UpdateCardParams{
		Title:         card.Title,
		Description:   card.Description,
		Position:      card.Position,
		DueDate:       card.DueDate,
		Priority:      card.Priority,
		IsArchived:    card.IsArchived,
		ArchivedAt:    card.ArchivedAt,
		ArchivedByID:  card.ArchivedByID,
		CompletedAt:   card.CompletedAt,
		CompletedByID: card.CompletedByID,
		ColumnID:      card.ColumnID,
		BorderColor:   card.BorderColor,
		ID:            card.ID,
	}
	mutate(&params)
	_, err := q.UpdateCard(ctx, params)
	return err
}

func parseUndoPlacement(s string) (columnID int64, pos float64, ok bool) {
	var raw struct {
		ColumnID int64   `json:"columnId"`
		Position float64 `json:"position"`
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil || raw.ColumnID == 0 {
		return 0, 0, false
	}
	return raw.ColumnID, raw.Position, true
}

func parseUndoPos(s string) (float64, bool) {
	pos, err := strconv.ParseFloat(s, 64)
	return pos, err == nil
}

func parseUndoID(s string) (int64, bool) {
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil && id != 0
}

func restoreCardAssignee(ctx context.Context, q *dbgen.Queries, cardID int64, before string) error {
	if err := q.ClearCardAssignees(ctx, cardID); err != nil {
		return err
	}
	if before == "" {
		return nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(before), &ids); err != nil {
		return undoImpossible()
	}
	for _, uid := range ids {
		if uid == 0 {
			continue
		}
		if err := q.AddCardAssignee(ctx, dbgen.AddCardAssigneeParams{CardID: cardID, UserID: uid}); err != nil {
			return err
		}
	}
	return nil
}

func parseUndoDue(s string) pgtype.Timestamptz {
	if s == "" {
		return pgtype.Timestamptz{}
	}
	t, err := time.ParseInLocation("02.01.2006 15:04", s, helper.MoscowLocation())
	if err != nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func textFromAny(raw any) pgtype.Text {
	if raw == nil {
		return pgtype.Text{}
	}
	if s, ok := raw.(string); ok && s != "" {
		return pgtype.Text{String: s, Valid: true}
	}
	return pgtype.Text{}
}
