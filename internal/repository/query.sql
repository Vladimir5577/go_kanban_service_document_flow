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

-- name: GetCardsByBoard :many
SELECT c.* FROM kanban_card c
JOIN kanban_column col ON col.id = c.column_id
WHERE col.board_id = $1 AND c.is_archived = FALSE
ORDER BY col.position ASC, c.position ASC;

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

-- name: GetCardAssigneesByCardIDs :many
SELECT card_id, user_id FROM kanban_card_assignee
WHERE card_id = ANY($1::bigint[]);

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

-- name: GetCardLabelsByCardIDs :many
SELECT kanban_card_id, kanban_label_id FROM kanban_card_label
WHERE kanban_card_id = ANY($1::bigint[]);

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

-- name: GetCommentCountsByCardIDs :many
SELECT card_id, COUNT(*) AS count
FROM kanban_card_comment
WHERE card_id = ANY($1::bigint[])
GROUP BY card_id;

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

-- name: GetSubtask :one
SELECT * FROM kanban_card_subtask
WHERE id = $1 LIMIT 1;

-- name: GetSubtasksByCard :many
SELECT * FROM kanban_card_subtask
WHERE card_id = $1
ORDER BY position ASC;

-- name: GetSubtaskCountsByCardIDs :many
SELECT card_id,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE LOWER(status) = 'done') AS done
FROM kanban_card_subtask
WHERE card_id = ANY($1::bigint[])
GROUP BY card_id;

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

-- name: GetChatAttachmentCountsByCardIDs :many
SELECT card_id, COUNT(*) AS count
FROM kanban_attachment
WHERE card_id = ANY($1::bigint[]) AND context = 'chat'
GROUP BY card_id;

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

-- name: DeleteProjectMembersExcept :exec
DELETE FROM kanban_project_user
WHERE kanban_project_id = $1 AND NOT (user_id = ANY(sqlc.arg(keep_user_ids)::bigint[]));

-- Новая строка участника всегда встаёт в конец личного списка (MAX+1 по user_id/folder_id).
-- Перемещение существующей строки — только через UpdateProjectPlacement.
-- ponytail: конкурентные вставки одного пользователя могут получить одинаковую позицию;
-- порядок добьёт tie-break по id и RebalanceProjectPositions при первом перетаскивании.
-- name: AddProjectMember :exec
INSERT INTO kanban_project_user (kanban_project_id, user_id, role, folder_id, position)
VALUES ($1, $2, $3, $4, COALESCE((
    SELECT MAX(p.position) FROM kanban_project_user p
    WHERE p.user_id = $2 AND p.folder_id IS NOT DISTINCT FROM $4
), 0) + 1)
ON CONFLICT (kanban_project_id, user_id) DO UPDATE
SET role = EXCLUDED.role;

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


-- ==============================
-- ASSIGNED TO ME (мои задачи / подзадачи)
-- ==============================

-- name: GetAssignedCardsOpen :many
SELECT
    p.id            AS project_id,
    p.name          AS project_name,
    b.id            AS board_id,
    b.title         AS board_title,
    b.position      AS board_position,
    col.id          AS column_id,
    col.title       AS column_title,
    col.position    AS column_position,
    c.id            AS card_id,
    c.title         AS card_title,
    c.priority      AS card_priority,
    c.due_date      AS card_due_date,
    c.border_color  AS card_border_color,
    c.position      AS card_position
FROM kanban_card_assignee ca
JOIN kanban_card    c   ON c.id  = ca.card_id
JOIN kanban_column  col ON col.id = c.column_id
JOIN kanban_board   b   ON b.id  = col.board_id
JOIN kanban_project p   ON p.id  = b.kanban_project_id
WHERE ca.user_id = $1
  AND c.completed_at IS NULL
  AND c.is_archived = FALSE
  AND b.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND (
      p.owner_id = $1
      OR EXISTS (
          SELECT 1 FROM kanban_project_user pu
          WHERE pu.kanban_project_id = p.id
            AND pu.user_id = $1
      )
  )
ORDER BY p.name, p.id, b.position, b.id, col.position, col.id, c.position, c.id;

-- name: GetAssignedCardsClosed :many
SELECT
    p.id            AS project_id,
    p.name          AS project_name,
    b.id            AS board_id,
    b.title         AS board_title,
    b.position      AS board_position,
    col.id          AS column_id,
    col.title       AS column_title,
    col.position    AS column_position,
    c.id            AS card_id,
    c.title         AS card_title,
    c.priority      AS card_priority,
    c.due_date      AS card_due_date,
    c.border_color  AS card_border_color,
    c.position      AS card_position
FROM kanban_card_assignee ca
JOIN kanban_card    c   ON c.id  = ca.card_id
JOIN kanban_column  col ON col.id = c.column_id
JOIN kanban_board   b   ON b.id  = col.board_id
JOIN kanban_project p   ON p.id  = b.kanban_project_id
WHERE ca.user_id = $1
  AND c.completed_at IS NOT NULL
  AND c.is_archived = FALSE
  AND b.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND (
      p.owner_id = $1
      OR EXISTS (
          SELECT 1 FROM kanban_project_user pu
          WHERE pu.kanban_project_id = p.id
            AND pu.user_id = $1
      )
  )
ORDER BY p.name, p.id, b.position, b.id, col.position, col.id, c.position, c.id;

-- name: GetAssignedSubtasksOpen :many
SELECT
    s.id       AS subtask_id,
    s.title    AS subtask_title,
    s.status   AS subtask_status,
    s.position AS subtask_position,
    c.id       AS card_id,
    c.title    AS card_title,
    col.id     AS column_id,
    col.title  AS column_title,
    b.id       AS board_id,
    b.title    AS board_title,
    p.id       AS project_id,
    p.name     AS project_name
FROM kanban_card_subtask s
JOIN kanban_card    c   ON c.id  = s.card_id
JOIN kanban_column  col ON col.id = c.column_id
JOIN kanban_board   b   ON b.id  = col.board_id
JOIN kanban_project p   ON p.id  = b.kanban_project_id
WHERE s.user_id = $1::bigint
  AND s.status <> 'done'
  AND b.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND (
      p.owner_id = $1::bigint
      OR EXISTS (
          SELECT 1 FROM kanban_project_user pu
          WHERE pu.kanban_project_id = p.id
            AND pu.user_id = $1::bigint
      )
  )
ORDER BY p.name, p.id, b.position, b.id, col.position, col.id, c.position, c.id, s.position, s.id;

-- name: GetAssignedSubtasksClosed :many
SELECT
    s.id       AS subtask_id,
    s.title    AS subtask_title,
    s.status   AS subtask_status,
    s.position AS subtask_position,
    c.id       AS card_id,
    c.title    AS card_title,
    col.id     AS column_id,
    col.title  AS column_title,
    b.id       AS board_id,
    b.title    AS board_title,
    p.id       AS project_id,
    p.name     AS project_name
FROM kanban_card_subtask s
JOIN kanban_card    c   ON c.id  = s.card_id
JOIN kanban_column  col ON col.id = c.column_id
JOIN kanban_board   b   ON b.id  = col.board_id
JOIN kanban_project p   ON p.id  = b.kanban_project_id
WHERE s.user_id = $1::bigint
  AND s.status = 'done'
  AND b.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND (
      p.owner_id = $1::bigint
      OR EXISTS (
          SELECT 1 FROM kanban_project_user pu
          WHERE pu.kanban_project_id = p.id
            AND pu.user_id = $1::bigint
      )
  )
ORDER BY p.name, p.id, b.position, b.id, col.position, col.id, c.position, c.id, s.position, s.id;
