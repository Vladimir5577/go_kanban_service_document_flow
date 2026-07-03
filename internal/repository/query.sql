-- name: GetProject :one
SELECT * FROM kanban_project
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetAllProjects :many
SELECT * FROM kanban_project
WHERE deleted_at IS NULL
ORDER BY id DESC;

-- name: CreateProject :one
INSERT INTO kanban_project (name, description, owner_id, created_by_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateProject :one
UPDATE kanban_project
SET name = $1, description = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $3 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteProject :exec
UPDATE kanban_project
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1;


-- ==============================
-- BOARDS
-- ==============================

-- name: GetBoard :one
SELECT * FROM kanban_board
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetBoardsByProject :many
SELECT * FROM kanban_board
WHERE kanban_project_id = $1 AND deleted_at IS NULL
ORDER BY position ASC;

-- name: CreateBoard :one
INSERT INTO kanban_board (title, position, kanban_project_id, created_by_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateBoard :one
UPDATE kanban_board
SET title = $1, position = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $3 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteBoard :exec
UPDATE kanban_board
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1;


-- ==============================
-- COLUMNS
-- ==============================

-- name: GetColumn :one
SELECT * FROM kanban_column
WHERE id = $1 LIMIT 1;

-- name: GetColumnsByBoard :many
SELECT * FROM kanban_column
WHERE board_id = $1
ORDER BY position ASC;

-- name: CreateColumn :one
INSERT INTO kanban_column (title, header_color, position, board_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateColumn :one
UPDATE kanban_column
SET title = $1, header_color = $2, position = $3
WHERE id = $4
RETURNING *;

-- name: DeleteColumn :exec
DELETE FROM kanban_column
WHERE id = $1;

-- name: HasCardsByColumn :one
SELECT EXISTS(
    SELECT 1 FROM kanban_card WHERE column_id = $1 AND is_archived = FALSE
);


-- ==============================
-- CARDS
-- ==============================

-- name: GetCard :one
SELECT * FROM kanban_card
WHERE id = $1 LIMIT 1;

-- name: GetCardsByColumn :many
SELECT * FROM kanban_card
WHERE column_id = $1 AND is_archived = FALSE
ORDER BY position ASC;

-- name: CreateCard :one
INSERT INTO kanban_card (title, description, position, due_date, priority, column_id, created_by_id, border_color)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: UpdateCard :one
UPDATE kanban_card
SET title = $1, description = $2, position = $3, due_date = $4, priority = $5, is_archived = $6, archived_at = $7, archived_by_id = $8, completed_at = $9, completed_by_id = $10, column_id = $11, border_color = $12, updated_at = CURRENT_TIMESTAMP
WHERE id = $13
RETURNING *;

-- name: DeleteCard :exec
DELETE FROM kanban_card
WHERE id = $1;

-- name: HasColumnsByBoard :one
SELECT EXISTS(
    SELECT 1 FROM kanban_column WHERE board_id = $1
);

-- name: RebalanceColumnCards :exec
WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER(ORDER BY position ASC, id ASC) as rn
  FROM kanban_card
  WHERE kanban_card.column_id = $1 AND kanban_card.is_archived = FALSE
)
UPDATE kanban_card
SET position = ranked.rn * 65536.0
FROM ranked
WHERE kanban_card.id = ranked.id;


-- ==============================
-- CARD ASSIGNEES
-- ==============================

-- name: GetCardAssignees :many
SELECT user_id FROM kanban_card_assignee
WHERE card_id = $1;

-- name: AddCardAssignee :exec
INSERT INTO kanban_card_assignee (card_id, user_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveCardAssignee :exec
DELETE FROM kanban_card_assignee
WHERE card_id = $1 AND user_id = $2;

-- name: ClearCardAssignees :exec
DELETE FROM kanban_card_assignee
WHERE card_id = $1;


-- ==============================
-- LABELS
-- ==============================

-- name: GetLabel :one
SELECT * FROM kanban_label
WHERE id = $1 LIMIT 1;

-- name: GetLabelsByBoard :many
SELECT * FROM kanban_label
WHERE board_id = $1;

-- name: CreateLabel :one
INSERT INTO kanban_label (name, color, board_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteLabel :exec
DELETE FROM kanban_label
WHERE id = $1;


-- ==============================
-- CARD LABELS
-- ==============================

-- name: GetCardLabels :many
SELECT kanban_label_id FROM kanban_card_label
WHERE kanban_card_id = $1;

-- name: AddCardLabel :exec
INSERT INTO kanban_card_label (kanban_card_id, kanban_label_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveCardLabel :exec
DELETE FROM kanban_card_label
WHERE kanban_card_id = $1 AND kanban_label_id = $2;


-- ==============================
-- COMMENTS
-- ==============================

-- name: GetComment :one
SELECT * FROM kanban_card_comment
WHERE id = $1 LIMIT 1;

-- name: GetCommentsByCard :many
SELECT * FROM kanban_card_comment
WHERE card_id = $1
ORDER BY created_at ASC;

-- name: CreateComment :one
INSERT INTO kanban_card_comment (body, card_id, author_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateComment :one
UPDATE kanban_card_comment
SET body = $1, updated_at = CURRENT_TIMESTAMP
WHERE id = $2
RETURNING *;

-- name: DeleteComment :exec
DELETE FROM kanban_card_comment
WHERE id = $1;


-- ==============================
-- SUBTASKS
-- ==============================

-- name: GetSubtasksByCard :many
SELECT * FROM kanban_card_subtask
WHERE card_id = $1
ORDER BY position ASC;

-- name: CreateSubtask :one
INSERT INTO kanban_card_subtask (title, status, position, card_id, user_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateSubtask :one
UPDATE kanban_card_subtask
SET title = $1, status = $2, position = $3, user_id = $4
WHERE id = $5
RETURNING *;

-- name: DeleteSubtask :exec
DELETE FROM kanban_card_subtask
WHERE id = $1;


-- ==============================
-- ATTACHMENTS
-- ==============================

-- name: GetAttachment :one
SELECT * FROM kanban_attachment
WHERE id = $1 LIMIT 1;

-- name: GetAttachmentsByCard :many
SELECT * FROM kanban_attachment
WHERE card_id = $1 AND context = $2
ORDER BY created_at ASC;

-- name: CreateAttachment :one
INSERT INTO kanban_attachment (filename, storage_key, content_type, size_bytes, context, card_id, author_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: DeleteAttachment :exec
DELETE FROM kanban_attachment
WHERE id = $1;


-- ==============================
-- ACTIVITY
-- ==============================

-- name: GetActivitiesByCard :many
SELECT * FROM kanban_card_activity
WHERE card_id = $1
ORDER BY created_at DESC;

-- name: CreateActivity :one
INSERT INTO kanban_card_activity (type, card_id, user_id, old_value, new_value)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;


-- ==============================
-- PROJECT FOLDERS
-- ==============================

-- name: GetProjectFolders :many
SELECT * FROM kanban_project_user_folder
WHERE user_id = $1 ORDER BY position ASC;

-- name: CreateProjectFolder :one
INSERT INTO kanban_project_user_folder (name, user_id, position)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateProjectFolder :one
UPDATE kanban_project_user_folder
SET name = $1, position = $2, updated_at = CURRENT_TIMESTAMP
WHERE id = $3
RETURNING *;

-- name: DeleteProjectFolder :exec
DELETE FROM kanban_project_user_folder
WHERE id = $1;


-- ==============================
-- PROJECT MEMBERS
-- ==============================

-- name: GetProjectMembers :many
SELECT * FROM kanban_project_user
WHERE kanban_project_id = $1;

-- name: ReplaceProjectMembers :exec
DELETE FROM kanban_project_user
WHERE kanban_project_id = $1;

-- name: AddProjectMember :exec
INSERT INTO kanban_project_user (kanban_project_id, user_id, role, folder_id, position)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (kanban_project_id, user_id) DO UPDATE 
SET role = EXCLUDED.role, folder_id = EXCLUDED.folder_id, position = EXCLUDED.position;

-- name: UpdateProjectMemberRole :exec
UPDATE kanban_project_user
SET role = $3
WHERE kanban_project_id = $1 AND user_id = $2;

-- name: GetProjectMember :one
SELECT id, kanban_project_id, user_id, role, folder_id, position FROM kanban_project_user
WHERE kanban_project_id = $1 AND user_id = $2;

-- name: GetProjectIDByColumn :one
SELECT b.kanban_project_id FROM kanban_column c
JOIN kanban_board b ON c.board_id = b.id
WHERE c.id = $1;

-- name: GetProjectIDBySubtask :one
SELECT b.kanban_project_id as project_id
FROM kanban_card_subtask s
JOIN kanban_card c ON s.card_id = c.id
JOIN kanban_column col ON c.column_id = col.id
JOIN kanban_board b ON col.board_id = b.id
WHERE s.id = $1;

-- name: GetProjectIDByLabel :one
SELECT b.kanban_project_id as project_id
FROM kanban_label l
JOIN kanban_board b ON l.board_id = b.id
WHERE l.id = $1;

-- name: GetProjectIDByCard :one
SELECT b.kanban_project_id FROM kanban_card card
JOIN kanban_column c ON card.column_id = c.id
JOIN kanban_board b ON c.board_id = b.id
WHERE card.id = $1;

-- name: RemoveProjectMember :exec
DELETE FROM kanban_project_user
WHERE kanban_project_id = $1 AND user_id = $2;
