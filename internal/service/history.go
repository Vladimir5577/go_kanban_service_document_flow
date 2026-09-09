package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"go_kanban_service/internal/model"
)

type HistoryLogger interface {
	Append(ctx context.Context, e model.HistoryWrite) error
}

func HistoryKey(kind string, id int64) string {
	return kind + ":" + strconv.FormatInt(id, 10)
}

func historyKeys(kind string, ids ...int64) []string {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			keys = append(keys, HistoryKey(kind, id))
		}
	}
	return keys
}

func appendHistory(j HistoryLogger, ctx context.Context, e model.HistoryWrite) {
	if j == nil || e.ProjectID == 0 {
		return
	}
	e.EntityTitle = strings.TrimSpace(e.EntityTitle)
	_ = j.Append(ctx, e)
}

func historyProjectPath(projectID int64) string {
	return "/projects/" + strconv.FormatInt(projectID, 10)
}

func historyBoardPath(projectID, boardID int64) string {
	return historyProjectPath(projectID) + "/board-" + strconv.FormatInt(boardID, 10)
}

func historyTaskPath(projectID, boardID, cardID int64) string {
	return historyBoardPath(projectID, boardID) + "/task-" + strconv.FormatInt(cardID, 10)
}

func historyArchivePath(projectID, boardID int64) string {
	return historyBoardPath(projectID, boardID) + "/archive"
}

func historyEditPath(projectID int64) string {
	return historyProjectPath(projectID) + "/edit"
}

func firstHistoryID(keys []string, kind string) int64 {
	prefix := kind + ":"
	for _, key := range keys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		id, err := strconv.ParseInt(key[len(prefix):], 10, 64)
		if err == nil && id != 0 {
			return id
		}
	}
	return 0
}

func historyText(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func clipHistory(s string) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > 80 {
		return string(r[:80]) + "…"
	}
	return s
}

func cardFieldAction(title, desc, due, priority, color bool) string {
	action := ""
	n := 0
	if title {
		n++
		action = "card.updated.renamed"
	}
	if desc {
		n++
		action = "card.updated.description"
	}
	if due {
		n++
		action = "card.updated.due_date"
	}
	if priority {
		n++
		action = "card.updated.priority"
	}
	if color {
		n++
		action = "card.updated.color"
	}
	if n != 1 {
		return "card.updated"
	}
	return action
}

func historyUserName(lastname, firstname, patronymic string) string {
	parts := compactHistoryName(lastname, firstname, patronymic)
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " ")
}

func compactHistoryName(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func cardPatchFields(c *model.Card) json.RawMessage {
	if c == nil {
		return nil
	}
	m := map[string]any{
		"title":         c.Title,
		"description":   nilString(c.Description),
		"priority":      nilString(c.Priority),
		"borderColor":   nilString(c.BorderColor),
		"dueDate":       nilTime(c.DueDate),
		"completedAt":   nilTime(c.CompletedAt),
		"completedById": nilInt(c.CompletedByID),
		"isArchived":    c.IsArchived,
		"archivedAt":    nilTime(c.ArchivedAt),
		"archivedById":  nilInt(c.ArchivedByID),
		"columnId":      c.ColumnID,
		"position":      c.Position,
	}
	b, _ := json.Marshal(m)
	return b
}

func nilString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func nilInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nilTime(v *time.Time) any {
	if v == nil {
		return nil
	}
	return v.Format(time.RFC3339)
}

func memberSnapshot(members []model.ProjectUser) json.RawMessage {
	type row struct {
		UserID   int64  `json:"userId"`
		Role     string `json:"role"`
		FolderID *int64 `json:"folderId,omitempty"`
		Position float64 `json:"position"`
	}
	rows := make([]row, 0, len(members))
	for _, m := range members {
		rows = append(rows, row{UserID: m.UserID, Role: m.Role, FolderID: m.FolderID, Position: m.Position})
	}
	b, _ := json.Marshal(rows)
	return b
}
