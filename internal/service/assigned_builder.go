package service

import (
	"go_kanban_service/internal/dto"
	"go_kanban_service/internal/repository"
)

func buildAssignedTree(rows []repository.AssignedCardRow) []*dto.AssignedProject {
	projects := make([]*dto.AssignedProject, 0)
	var curProject *dto.AssignedProject
	var curBoard *dto.AssignedBoard
	var curColumn *dto.AssignedColumn

	for _, row := range rows {
		if curProject == nil || curProject.ID != row.ProjectID {
			curProject = &dto.AssignedProject{
				ID:     row.ProjectID,
				Name:   row.ProjectName,
				Boards: make([]*dto.AssignedBoard, 0),
			}
			projects = append(projects, curProject)
			curBoard, curColumn = nil, nil
		}
		if curBoard == nil || curBoard.ID != row.BoardID {
			curBoard = &dto.AssignedBoard{
				ID:      row.BoardID,
				Title:   row.BoardTitle,
				Columns: make([]*dto.AssignedColumn, 0),
			}
			curProject.Boards = append(curProject.Boards, curBoard)
			curColumn = nil
		}
		if curColumn == nil || curColumn.ID != row.ColumnID {
			curColumn = &dto.AssignedColumn{
				ID:    row.ColumnID,
				Title: row.ColumnTitle,
				Cards: make([]*dto.AssignedCard, 0),
			}
			curBoard.Columns = append(curBoard.Columns, curColumn)
		}

		curColumn.Cards = append(curColumn.Cards, &dto.AssignedCard{
			ID:          row.CardID,
			Title:       row.CardTitle,
			Priority:    row.Priority,
			DueDate:     row.DueDate,
			BorderColor: row.BorderColor,
		})
	}

	return projects
}

func mapAssignedSubtasks(rows []repository.AssignedSubtaskRow) []*dto.AssignedSubtask {
	subtasks := make([]*dto.AssignedSubtask, 0, len(rows))
	for _, row := range rows {
		subtasks = append(subtasks, &dto.AssignedSubtask{
			ID:     row.SubtaskID,
			Title:  row.SubtaskTitle,
			Status: row.SubtaskStatus,
			Card: dto.AssignedSubtaskCardRef{
				ID:    row.CardID,
				Title: row.CardTitle,
			},
			Column: dto.AssignedSubtaskColumnRef{
				ID:    row.ColumnID,
				Title: row.ColumnTitle,
			},
			Board: dto.AssignedSubtaskBoardRef{
				ID:    row.BoardID,
				Title: row.BoardTitle,
			},
			Project: dto.AssignedSubtaskProjectRef{
				ID:   row.ProjectID,
				Name: row.ProjectName,
			},
		})
	}
	return subtasks
}
