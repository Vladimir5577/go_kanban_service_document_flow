package dto

import "time"

type AssignedCard struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Priority    *string    `json:"priority"`
	DueDate     *time.Time `json:"dueDate"`
	BorderColor *string    `json:"borderColor"`
}

type AssignedColumn struct {
	ID    int64           `json:"id"`
	Title string          `json:"title"`
	Cards []*AssignedCard `json:"cards"`
}

type AssignedBoard struct {
	ID      int64             `json:"id"`
	Title   string            `json:"title"`
	Columns []*AssignedColumn `json:"columns"`
}

type AssignedProject struct {
	ID     int64            `json:"id"`
	Name   string           `json:"name"`
	Boards []*AssignedBoard `json:"boards"`
}

type AssignedSubtaskCardRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type AssignedSubtaskColumnRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type AssignedSubtaskBoardRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

type AssignedSubtaskProjectRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type AssignedSubtask struct {
	ID      int64                     `json:"id"`
	Title   string                    `json:"title"`
	Status  string                    `json:"status"`
	Card    AssignedSubtaskCardRef    `json:"card"`
	Column  AssignedSubtaskColumnRef  `json:"column"`
	Board   AssignedSubtaskBoardRef   `json:"board"`
	Project AssignedSubtaskProjectRef `json:"project"`
}

type AssignedToMeResponse struct {
	AssignedCards    []*AssignedProject `json:"assignedCards"`
	AssignedSubtasks []*AssignedSubtask `json:"assignedSubtasks"`
}
