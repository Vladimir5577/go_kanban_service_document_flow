package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository/dbgen"
)

func applyUndoStep(ctx context.Context, q *dbgen.Queries, step model.UndoStep) error {
	switch step.Op {
	case "card.move":
		card, err := q.GetCard(ctx, step.CardID)
		if err != nil {
			return err
		}
		return updateCardRow(ctx, q, card, func(p *dbgen.UpdateCardParams) {
			if step.ColumnID != 0 {
				p.ColumnID = step.ColumnID
			}
			if step.Position != nil {
				p.Position = *step.Position
			}
		})
	case "card.patch":
		card, err := q.GetCard(ctx, step.CardID)
		if err != nil {
			return err
		}
		var fields map[string]any
		if err := json.Unmarshal(step.Fields, &fields); err != nil {
			return err
		}
		return updateCardRow(ctx, q, card, func(p *dbgen.UpdateCardParams) {
			applyCardFields(p, fields)
		})
	case "card.soft_delete":
		return q.DeleteCard(ctx, step.CardID)
	case "card.restore":
		return q.RestoreCard(ctx, step.CardID)
	case "card.assignees":
		if err := q.ClearCardAssignees(ctx, step.CardID); err != nil {
			return err
		}
		for _, uid := range step.UserIDs {
			if err := q.AddCardAssignee(ctx, dbgen.AddCardAssigneeParams{CardID: step.CardID, UserID: uid}); err != nil {
				return err
			}
		}
		return nil
	case "board.soft_delete":
		return q.DeleteBoard(ctx, step.BoardID)
	case "board.restore":
		return q.RestoreBoard(ctx, step.BoardID)
	case "board.patch":
		board, err := q.GetBoard(ctx, step.BoardID)
		if err != nil {
			return err
		}
		var fields map[string]any
		if len(step.Fields) > 0 {
			if err := json.Unmarshal(step.Fields, &fields); err != nil {
				return err
			}
		}
		title := board.Title
		position := board.Position
		if v, ok := fields["title"].(string); ok {
			title = v
		}
		if v, ok := fields["position"].(float64); ok {
			position = v
		}
		_, err = q.UpdateBoard(ctx, dbgen.UpdateBoardParams{
			Title:    title,
			Position: position,
			ID:       board.ID,
		})
		if err != nil {
			return err
		}
		if raw, ok := fields["doneColumnId"]; ok {
			var doneID *int64
			switch n := raw.(type) {
			case float64:
				v := int64(n)
				doneID = &v
			case nil:
				doneID = nil
			}
			arg := pgtype.Int8{}
			if doneID != nil {
				arg = pgtype.Int8{Int64: *doneID, Valid: true}
			}
			return q.SetDoneColumnID(ctx, dbgen.SetDoneColumnIDParams{DoneColumnID: arg, ID: board.ID})
		}
		return nil
	case "project.soft_delete":
		return q.DeleteProject(ctx, step.ProjectID)
	case "project.restore":
		return q.RestoreProject(ctx, step.ProjectID)
	case "project.patch":
		p, err := q.GetProject(ctx, step.ProjectID)
		if err != nil {
			return err
		}
		var fields map[string]any
		if err := json.Unmarshal(step.Fields, &fields); err != nil {
			return err
		}
		name := p.Name
		desc := p.Description
		if v, ok := fields["name"].(string); ok {
			name = v
		}
		if raw, ok := fields["description"]; ok {
			if raw == nil {
				desc = pgtype.Text{}
			} else if s, ok := raw.(string); ok {
				desc = pgtype.Text{String: s, Valid: true}
			}
		}
		_, err = q.UpdateProject(ctx, dbgen.UpdateProjectParams{Name: name, Description: desc, ID: p.ID})
		return err
	case "column.delete":
		return q.DeleteColumn(ctx, step.ColumnID)
	case "column.insert":
		var snap struct {
			ID          int64   `json:"id"`
			Title       string  `json:"title"`
			HeaderColor string  `json:"headerColor"`
			Position    float64 `json:"position"`
			BoardID     int64   `json:"boardId"`
		}
		if err := json.Unmarshal(step.Snapshot, &snap); err != nil {
			return err
		}
		if _, err := q.InsertColumnWithID(ctx, dbgen.InsertColumnWithIDParams{
			ID:          snap.ID,
			Title:       snap.Title,
			HeaderColor: snap.HeaderColor,
			Position:    snap.Position,
			BoardID:     snap.BoardID,
		}); err != nil {
			return err
		}
		return q.SyncColumnIDSeq(ctx)
	case "column.patch":
		col, err := q.GetColumn(ctx, step.ColumnID)
		if err != nil {
			return err
		}
		var fields map[string]any
		if err := json.Unmarshal(step.Fields, &fields); err != nil {
			return err
		}
		title := col.Title
		color := col.HeaderColor
		pos := col.Position
		if v, ok := fields["title"].(string); ok {
			title = v
		}
		if v, ok := fields["headerColor"].(string); ok {
			color = v
		}
		if v, ok := fields["position"].(float64); ok {
			pos = v
		}
		_, err = q.UpdateColumn(ctx, dbgen.UpdateColumnParams{
			Title:       title,
			HeaderColor: color,
			Position:    pos,
			ID:          col.ID,
		})
		return err
	case "label.delete":
		return q.DeleteLabel(ctx, step.LabelID)
	case "label.insert":
		var snap struct {
			ID      int64   `json:"id"`
			Name    string  `json:"name"`
			Color   string  `json:"color"`
			BoardID int64   `json:"boardId"`
			CardIDs []int64 `json:"cardIds"`
		}
		if err := json.Unmarshal(step.Snapshot, &snap); err != nil {
			return err
		}
		if _, err := q.InsertLabelWithID(ctx, dbgen.InsertLabelWithIDParams{
			ID: snap.ID, Name: snap.Name, Color: snap.Color, BoardID: snap.BoardID,
		}); err != nil {
			return err
		}
		if err := q.SyncLabelIDSeq(ctx); err != nil {
			return err
		}
		for _, cardID := range snap.CardIDs {
			if err := q.AddCardLabel(ctx, dbgen.AddCardLabelParams{KanbanCardID: cardID, KanbanLabelID: snap.ID}); err != nil {
				return err
			}
		}
		return nil
	case "label.toggle":
		if step.On != nil && *step.On {
			return q.AddCardLabel(ctx, dbgen.AddCardLabelParams{KanbanCardID: step.CardID, KanbanLabelID: step.LabelID})
		}
		return q.RemoveCardLabel(ctx, dbgen.RemoveCardLabelParams{KanbanCardID: step.CardID, KanbanLabelID: step.LabelID})
	case "comment.delete":
		return q.DeleteComment(ctx, step.CommentID)
	case "comment.insert":
		var snap struct {
			ID        int64     `json:"id"`
			Body      string    `json:"body"`
			CardID    int64     `json:"cardId"`
			AuthorID  int64     `json:"authorId"`
			CreatedAt time.Time `json:"createdAt"`
		}
		if err := json.Unmarshal(step.Snapshot, &snap); err != nil {
			return err
		}
		if _, err := q.InsertCommentWithID(ctx, dbgen.InsertCommentWithIDParams{
			ID:        snap.ID,
			Body:      snap.Body,
			CardID:    snap.CardID,
			AuthorID:  snap.AuthorID,
			CreatedAt: pgtype.Timestamptz{Time: snap.CreatedAt, Valid: true},
		}); err != nil {
			return err
		}
		return q.SyncCommentIDSeq(ctx)
	case "comment.patch":
		var fields struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(step.Fields, &fields); err != nil {
			return err
		}
		_, err := q.UpdateComment(ctx, dbgen.UpdateCommentParams{Body: fields.Body, ID: step.CommentID})
		return err
	case "subtask.delete":
		return q.DeleteSubtask(ctx, step.SubtaskID)
	case "subtask.insert":
		var snap struct {
			ID       int64   `json:"id"`
			Title    string  `json:"title"`
			Status   string  `json:"status"`
			Position float64 `json:"position"`
			CardID   int64   `json:"cardId"`
			UserID   *int64  `json:"userId"`
		}
		if err := json.Unmarshal(step.Snapshot, &snap); err != nil {
			return err
		}
		params := dbgen.InsertSubtaskWithIDParams{
			ID: snap.ID, Title: snap.Title, Status: snap.Status, Position: snap.Position, CardID: snap.CardID,
		}
		if snap.UserID != nil {
			params.UserID = pgtype.Int8{Int64: *snap.UserID, Valid: true}
		}
		if _, err := q.InsertSubtaskWithID(ctx, params); err != nil {
			return err
		}
		return q.SyncSubtaskIDSeq(ctx)
	case "subtask.patch":
		st, err := q.GetSubtask(ctx, step.SubtaskID)
		if err != nil {
			return err
		}
		var fields map[string]any
		if err := json.Unmarshal(step.Fields, &fields); err != nil {
			return err
		}
		title := st.Title
		status := st.Status
		pos := st.Position
		userID := st.UserID
		if v, ok := fields["title"].(string); ok {
			title = v
		}
		if v, ok := fields["status"].(string); ok {
			status = v
		}
		if v, ok := fields["position"].(float64); ok {
			pos = v
		}
		if raw, ok := fields["userId"]; ok {
			if raw == nil {
				userID = pgtype.Int8{}
			} else if n, ok := raw.(float64); ok {
				userID = pgtype.Int8{Int64: int64(n), Valid: true}
			}
		}
		_, err = q.UpdateSubtask(ctx, dbgen.UpdateSubtaskParams{
			Title: title, Status: status, Position: pos, UserID: userID, ID: st.ID,
		})
		return err
	case "attachment.soft_delete":
		return q.DeleteAttachment(ctx, step.AttachID)
	case "attachment.restore":
		return q.RestoreAttachment(ctx, step.AttachID)
	case "members.replace":
		return nil
	default:
		return fmt.Errorf("unknown undo op %q", step.Op)
	}
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

func applyCardFields(p *dbgen.UpdateCardParams, fields map[string]any) {
	if v, ok := fields["title"].(string); ok {
		p.Title = v
	}
	if raw, ok := fields["description"]; ok {
		p.Description = textFromAny(raw)
	}
	if raw, ok := fields["priority"]; ok {
		p.Priority = textFromAny(raw)
	}
	if raw, ok := fields["borderColor"]; ok {
		p.BorderColor = textFromAny(raw)
	}
	if raw, ok := fields["dueDate"]; ok {
		p.DueDate = timestamptzFromAny(raw)
	}
	if raw, ok := fields["completedAt"]; ok {
		p.CompletedAt = timestamptzFromAny(raw)
	}
	if raw, ok := fields["completedById"]; ok {
		p.CompletedByID = int8FromAny(raw)
	}
	if raw, ok := fields["isArchived"].(bool); ok {
		p.IsArchived = raw
	}
	if raw, ok := fields["archivedAt"]; ok {
		p.ArchivedAt = timestamptzFromAny(raw)
	}
	if raw, ok := fields["archivedById"]; ok {
		p.ArchivedByID = int8FromAny(raw)
	}
	if v, ok := fields["columnId"].(float64); ok {
		p.ColumnID = int64(v)
	}
	if v, ok := fields["position"].(float64); ok {
		p.Position = v
	}
}

func textFromAny(raw any) pgtype.Text {
	if raw == nil {
		return pgtype.Text{}
	}
	if s, ok := raw.(string); ok {
		return pgtype.Text{String: s, Valid: true}
	}
	return pgtype.Text{}
}

func int8FromAny(raw any) pgtype.Int8 {
	if raw == nil {
		return pgtype.Int8{}
	}
	if n, ok := raw.(float64); ok {
		return pgtype.Int8{Int64: int64(n), Valid: true}
	}
	return pgtype.Int8{}
}

func timestamptzFromAny(raw any) pgtype.Timestamptz {
	if raw == nil {
		return pgtype.Timestamptz{}
	}
	if s, ok := raw.(string); ok && s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return pgtype.Timestamptz{Time: t, Valid: true}
		}
	}
	return pgtype.Timestamptz{}
}
