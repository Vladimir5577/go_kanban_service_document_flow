package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"go_kanban_service/internal/model"
)

type HistoryLogger interface {
	Append(ctx context.Context, e model.HistoryWrite) error
}

func appendHistory(j HistoryLogger, ctx context.Context, e model.HistoryWrite) {
	if j == nil || e.ProjectID == 0 || e.EntityID == 0 {
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

func historyText(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func historyLabelCardsCount(n int) string {
	b, err := json.Marshal(struct {
		LabelCardsCount int `json:"labelCardsCount"`
	}{LabelCardsCount: n})
	if err != nil {
		return ""
	}
	return string(b)
}

func historyPlacement(columnID int64, pos float64) string {
	b, err := json.Marshal(struct {
		ColumnID int64   `json:"columnId"`
		Position float64 `json:"position"`
	}{ColumnID: columnID, Position: pos})
	if err != nil {
		return ""
	}
	return string(b)
}

func historyPos(pos float64) string {
	return strconv.FormatFloat(pos, 'f', -1, 64)
}

func historyOptID(id *int64) string {
	if id == nil {
		return ""
	}
	return strconv.FormatInt(*id, 10)
}

func historyIDsJSON(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return ""
	}
	return string(b)
}

type subtaskUpdateSummary struct {
	titleChanged    bool
	prevTitle       string
	newTitle        string
	statusChanged   bool
	nowCompleted    bool
	posChanged      bool
	oldPos          float64
	newPos          float64
	assigneeChanged bool
	oldUserID       *int64
	newUserID       *int64
}

func subtaskUpdateAction(s subtaskUpdateSummary) (action, before, after string) {
	n := 0
	if s.titleChanged {
		n++
		action, before, after = "subtask.renamed", s.prevTitle, s.newTitle
	}
	if s.statusChanged {
		n++
		if s.nowCompleted {
			action = "subtask.completed"
		} else {
			action = "subtask.reopened"
		}
		before, after = "", ""
	}
	if s.posChanged {
		n++
		action, before, after = "subtask.moved", historyPos(s.oldPos), historyPos(s.newPos)
	}
	if s.assigneeChanged {
		n++
		before, after = historyOptID(s.oldUserID), historyOptID(s.newUserID)
		if s.newUserID != nil {
			action = "subtask.assigned"
		} else {
			action = "subtask.unassigned"
		}
	}
	if n != 1 {
		return "subtask.updated", "", ""
	}
	return action, before, after
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

func membersHistoryJSON(members []model.ProjectUser) string {
	type row struct {
		UserID   int64   `json:"userId"`
		Role     string  `json:"role"`
		FolderID *int64  `json:"folderId,omitempty"`
		Position float64 `json:"position"`
	}
	rows := make([]row, 0, len(members))
	for _, m := range members {
		rows = append(rows, row{UserID: m.UserID, Role: m.Role, FolderID: m.FolderID, Position: m.Position})
	}
	b, err := json.Marshal(rows)
	if err != nil {
		return ""
	}
	return string(b)
}
