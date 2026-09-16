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

-- name: SetDoneColumnID :exec
UPDATE kanban_board
SET done_column_id = $1, updated_at = CURRENT_TIMESTAMP
WHERE id = $2 AND deleted_at IS NULL;

-- name: DeleteBoard :exec
UPDATE kanban_board
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1;


-- ==============================
-- COLUMNS
-- ==============================

-- name: GetColumn :one
SELECT * FROM kanban_column
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetColumnsByBoard :many
SELECT * FROM kanban_column
WHERE board_id = $1 AND deleted_at IS NULL
ORDER BY position ASC;

-- name: CreateColumn :one
INSERT INTO kanban_column (title, header_color, position, board_id)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateColumn :one
UPDATE kanban_column
SET title = $1, header_color = $2, position = $3
WHERE id = $4 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteColumn :exec
UPDATE kanban_column
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;

-- name: RestoreColumn :exec
UPDATE kanban_column
SET deleted_at = NULL
WHERE id = $1;

-- name: HasCardsByColumn :one
SELECT EXISTS(
    SELECT 1 FROM kanban_card
    WHERE column_id = $1 AND parent_id IS NULL AND deleted_at IS NULL
);


-- ==============================
-- CARDS
-- ==============================

-- name: GetCard :one
SELECT * FROM kanban_card
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetCardsByColumn :many
SELECT * FROM kanban_card
WHERE column_id = $1 AND parent_id IS NULL AND is_archived = FALSE AND deleted_at IS NULL
ORDER BY position ASC;

-- name: GetCardsByBoard :many
SELECT c.* FROM kanban_card c
JOIN kanban_column col ON col.id = c.column_id
WHERE col.board_id = $1 AND c.parent_id IS NULL AND c.is_archived = FALSE AND c.deleted_at IS NULL AND col.deleted_at IS NULL
ORDER BY col.position ASC, c.position ASC;

-- name: GetChildCards :many
SELECT * FROM kanban_card
WHERE parent_id = $1 AND deleted_at IS NULL
ORDER BY position ASC;

-- name: GetChildCountsByParentIDs :many
SELECT parent_id,
       COUNT(*) AS total,
       COUNT(*) FILTER (WHERE completed_at IS NOT NULL) AS done
FROM kanban_card
WHERE parent_id = ANY($1::bigint[]) AND deleted_at IS NULL
GROUP BY parent_id;

-- name: CreateCard :one
INSERT INTO kanban_card (title, description, position, due_date, priority, column_id, parent_id, created_by_id, border_color)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: UpdateCard :one
UPDATE kanban_card
SET title = $1, description = $2, position = $3, due_date = $4, priority = $5, is_archived = $6, archived_at = $7, archived_by_id = $8, completed_at = $9, completed_by_id = $10, column_id = $11, parent_id = $12, border_color = $13, updated_at = CURRENT_TIMESTAMP
WHERE id = $14
RETURNING *;

-- name: DeleteCard :exec
UPDATE kanban_card
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL AND (id = $1 OR parent_id = $1);

-- name: RestoreCard :exec
UPDATE kanban_card
SET deleted_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = $1 OR parent_id = $1;

-- name: HasColumnsByBoard :one
SELECT EXISTS(
    SELECT 1 FROM kanban_column WHERE board_id = $1 AND deleted_at IS NULL
);

-- name: ColumnPositionTaken :one
-- Занята ли позиция в целевой колонке. Допуск нужен потому, что позиции дробные
-- и приходят от клиента, — точное равенство double precision тут не работает.
-- EXISTS выходит на первом совпадении: раньше ради ответа «да/нет» читались и
-- ехали по сети все карточки колонки целиком.
-- Фильтр тот же, что был у прежней проверки, — только is_archived.
SELECT EXISTS(
    SELECT 1 FROM kanban_card
    WHERE column_id = $1 AND parent_id IS NULL AND id <> $2 AND is_archived = FALSE
      AND abs(position - sqlc.arg(position)::double precision) < 0.0001
);

-- name: RebalanceColumnCards :exec
WITH ranked AS (
  SELECT id, ROW_NUMBER() OVER(ORDER BY position ASC, id ASC) as rn
  FROM kanban_card
  WHERE kanban_card.column_id = $1 AND kanban_card.parent_id IS NULL AND kanban_card.is_archived = FALSE AND kanban_card.deleted_at IS NULL
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
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetLabelsByBoard :many
SELECT * FROM kanban_label
WHERE board_id = $1 AND deleted_at IS NULL;

-- name: CreateLabel :one
INSERT INTO kanban_label (name, color, board_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteLabel :exec
UPDATE kanban_label
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;

-- name: RestoreLabel :exec
UPDATE kanban_label
SET deleted_at = NULL
WHERE id = $1;

-- name: GetCardIDsByLabel :many
SELECT cl.kanban_card_id FROM kanban_card_label cl
JOIN kanban_card c ON c.id = cl.kanban_card_id
WHERE cl.kanban_label_id = $1 AND c.deleted_at IS NULL;


-- ==============================
-- CARD LABELS
-- ==============================

-- name: GetCardLabels :many
SELECT cl.kanban_label_id FROM kanban_card_label cl
JOIN kanban_label l ON l.id = cl.kanban_label_id
WHERE cl.kanban_card_id = $1 AND l.deleted_at IS NULL;

-- name: GetCardLabelsByCardIDs :many
SELECT cl.kanban_card_id, cl.kanban_label_id FROM kanban_card_label cl
JOIN kanban_label l ON l.id = cl.kanban_label_id
WHERE cl.kanban_card_id = ANY($1::bigint[]) AND l.deleted_at IS NULL;

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
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetCommentsByCard :many
SELECT * FROM kanban_card_comment
WHERE card_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetCommentCountsByCardIDs :many
SELECT card_id, COUNT(*) AS count
FROM kanban_card_comment
WHERE card_id = ANY($1::bigint[]) AND deleted_at IS NULL
GROUP BY card_id;

-- name: CreateComment :one
INSERT INTO kanban_card_comment (body, card_id, author_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateComment :one
UPDATE kanban_card_comment
SET body = $1, updated_at = CURRENT_TIMESTAMP
WHERE id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteComment :exec
UPDATE kanban_card_comment
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;

-- name: RestoreComment :exec
UPDATE kanban_card_comment
SET deleted_at = NULL
WHERE id = $1;


-- ==============================
-- ATTACHMENTS
-- ==============================

-- name: GetAttachment :one
SELECT * FROM kanban_attachment
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: GetAttachmentsByCard :many
SELECT * FROM kanban_attachment
WHERE card_id = $1 AND context = $2 AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetChatAttachmentCountsByCardIDs :many
SELECT card_id, COUNT(*) AS count
FROM kanban_attachment
WHERE card_id = ANY($1::bigint[]) AND context = 'chat' AND deleted_at IS NULL
GROUP BY card_id;

-- name: CreateAttachment :one
INSERT INTO kanban_attachment (filename, storage_key, content_type, size_bytes, context, card_id, author_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: DeleteAttachment :exec
UPDATE kanban_attachment
SET deleted_at = CURRENT_TIMESTAMP
WHERE id = $1 AND deleted_at IS NULL;

-- name: RestoreAttachment :exec
UPDATE kanban_attachment
SET deleted_at = NULL
WHERE id = $1;


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

-- name: GetProjectIDByLabel :one
SELECT b.kanban_project_id as project_id
FROM kanban_label l
JOIN kanban_board b ON l.board_id = b.id
WHERE l.id = $1;

-- name: GetProjectIDByCard :one
SELECT b.kanban_project_id FROM kanban_card card
LEFT JOIN kanban_card parent ON parent.id = card.parent_id
JOIN kanban_column c ON c.id = COALESCE(card.column_id, parent.column_id)
JOIN kanban_board b ON c.board_id = b.id
WHERE card.id = $1;

-- name: GetCardContext :one
-- Всё, что нужно для проверки прав и для шапки карточки, одним запросом:
-- проект, доска с названием, заголовок колонки, владелец и роль вызывающего.
-- Джойны те же, что в GetProjectIDByCard, плюс проект — все по первичным
-- ключам, членство — по уникальному (kanban_project_id, user_id).
-- Роль берём LEFT JOIN'ом, а не отдельным GetProjectMember: членства может не
-- быть вовсе (владелец, чужой пользователь), и NULL здесь — это отказ, который
-- разбирает resolveRole.
-- Название доски нужно уведомлениям: без него они ходили за ним в GetBoard.
-- deleted_at проекта не фильтруем в WHERE, а возвращаем: иначе «карточки нет»
-- и «проект удалён» схлопнутся в одну ошибку и фронт получит не тот код.
SELECT
    b.kanban_project_id,
    b.id AS board_id,
    b.title AS board_title,
    col.title AS column_title,
    p.owner_id,
    p.deleted_at AS project_deleted_at,
    pu.role AS member_role,
    card.parent_id AS parent_id
FROM kanban_card card
LEFT JOIN kanban_card parent ON parent.id = card.parent_id
JOIN kanban_column col ON col.id = COALESCE(card.column_id, parent.column_id)
JOIN kanban_board b ON col.board_id = b.id
JOIN kanban_project p ON b.kanban_project_id = p.id
LEFT JOIN kanban_project_user pu
       ON pu.kanban_project_id = p.id AND pu.user_id = $2
WHERE card.id = $1;

-- name: RemoveProjectMember :exec
DELETE FROM kanban_project_user
WHERE kanban_project_id = $1 AND user_id = $2;


-- ==============================
-- TASK LIST
-- ==============================

-- name: CountTasks :one
SELECT COUNT(*)::bigint AS count
FROM kanban_card c
LEFT JOIN kanban_card parent ON parent.id = c.parent_id
JOIN kanban_column col ON col.id = COALESCE(c.column_id, parent.column_id)
JOIN kanban_board b ON b.id = col.board_id
JOIN kanban_project p ON p.id = b.kanban_project_id
WHERE c.deleted_at IS NULL
  AND (parent.id IS NULL OR parent.deleted_at IS NULL)
  AND col.deleted_at IS NULL
  AND b.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND (
      p.owner_id = sqlc.arg(viewer_id)
      OR EXISTS (
          SELECT 1 FROM kanban_project_user pu
          WHERE pu.kanban_project_id = p.id
            AND pu.user_id = sqlc.arg(viewer_id)
      )
  )
  AND (sqlc.arg(title_query)::text = '' OR c.title ILIKE '%' || sqlc.arg(title_query) || '%' ESCAPE '\')
  AND (sqlc.arg(project_id)::bigint = 0 OR p.id = sqlc.arg(project_id))
  AND (
      sqlc.arg(priority)::text = ''
      OR (sqlc.arg(priority) = 'none' AND c.priority IS NULL)
      OR (sqlc.arg(priority) <> 'none' AND c.priority = sqlc.arg(priority))
  )
  AND (
      sqlc.arg(author_id)::bigint = 0
      OR (sqlc.arg(author_id) < 0 AND c.created_by_id IS NULL)
      OR (sqlc.arg(author_id) > 0 AND c.created_by_id = sqlc.arg(author_id))
  )
  AND (
      sqlc.arg(assignee_id)::bigint = 0
      OR (
          sqlc.arg(assignee_id) < 0
          AND NOT EXISTS (SELECT 1 FROM kanban_card_assignee ca WHERE ca.card_id = c.id)
      )
      OR (
          sqlc.arg(assignee_id) > 0
          AND EXISTS (
              SELECT 1 FROM kanban_card_assignee ca
              WHERE ca.card_id = c.id AND ca.user_id = sqlc.arg(assignee_id)
          )
      )
  )
  AND (
      sqlc.arg(completed)::text = ''
      OR (c.completed_at IS NOT NULL) = (sqlc.arg(completed) = 'true')
  )
  AND (
      sqlc.arg(archived)::text = ''
      OR (
          CASE WHEN c.parent_id IS NULL THEN c.is_archived ELSE COALESCE(parent.is_archived, FALSE) END
      ) = (sqlc.arg(archived) = 'true')
  )
  AND (
      sqlc.arg(kind)::text = ''
      OR (sqlc.arg(kind) = 'task' AND c.parent_id IS NULL)
      OR (sqlc.arg(kind) = 'subtask' AND c.parent_id IS NOT NULL)
  )
  AND (sqlc.narg('due_from')::timestamptz IS NULL OR c.due_date >= sqlc.narg('due_from'))
  AND (sqlc.narg('due_to')::timestamptz IS NULL OR c.due_date < sqlc.narg('due_to'))
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR c.created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR c.created_at < sqlc.narg('created_to'))
  AND (sqlc.narg('completed_from')::timestamptz IS NULL OR c.completed_at >= sqlc.narg('completed_from'))
  AND (sqlc.narg('completed_to')::timestamptz IS NULL OR c.completed_at < sqlc.narg('completed_to'))
  AND (
      sqlc.narg('archived_from')::timestamptz IS NULL
      OR (CASE WHEN c.parent_id IS NULL THEN c.archived_at ELSE parent.archived_at END) >= sqlc.narg('archived_from')
  )
  AND (
      sqlc.narg('archived_to')::timestamptz IS NULL
      OR (CASE WHEN c.parent_id IS NULL THEN c.archived_at ELSE parent.archived_at END) < sqlc.narg('archived_to')
  );

-- name: ListTasks :many
SELECT
    p.id            AS project_id,
    p.name          AS project_name,
    b.id            AS board_id,
    b.title         AS board_title,
    col.id          AS column_id,
    col.title       AS column_title,
    c.id            AS card_id,
    c.title         AS card_title,
    c.priority      AS card_priority,
    c.due_date      AS card_due_date,
    c.border_color  AS card_border_color,
    c.parent_id     AS parent_id,
    parent.title    AS parent_title,
    c.created_at    AS created_at,
    c.completed_at  AS completed_at,
    (CASE WHEN c.parent_id IS NULL THEN c.archived_at ELSE parent.archived_at END)::timestamptz AS archived_at,
    (CASE WHEN c.parent_id IS NULL THEN c.is_archived ELSE COALESCE(parent.is_archived, FALSE) END)::bool AS is_archived
FROM kanban_card c
LEFT JOIN kanban_card parent ON parent.id = c.parent_id
JOIN kanban_column col ON col.id = COALESCE(c.column_id, parent.column_id)
JOIN kanban_board b ON b.id = col.board_id
JOIN kanban_project p ON p.id = b.kanban_project_id
WHERE c.deleted_at IS NULL
  AND (parent.id IS NULL OR parent.deleted_at IS NULL)
  AND col.deleted_at IS NULL
  AND b.deleted_at IS NULL
  AND p.deleted_at IS NULL
  AND (
      p.owner_id = sqlc.arg(viewer_id)
      OR EXISTS (
          SELECT 1 FROM kanban_project_user pu
          WHERE pu.kanban_project_id = p.id
            AND pu.user_id = sqlc.arg(viewer_id)
      )
  )
  AND (sqlc.arg(title_query)::text = '' OR c.title ILIKE '%' || sqlc.arg(title_query) || '%' ESCAPE '\')
  AND (sqlc.arg(project_id)::bigint = 0 OR p.id = sqlc.arg(project_id))
  AND (
      sqlc.arg(priority)::text = ''
      OR (sqlc.arg(priority) = 'none' AND c.priority IS NULL)
      OR (sqlc.arg(priority) <> 'none' AND c.priority = sqlc.arg(priority))
  )
  AND (
      sqlc.arg(author_id)::bigint = 0
      OR (sqlc.arg(author_id) < 0 AND c.created_by_id IS NULL)
      OR (sqlc.arg(author_id) > 0 AND c.created_by_id = sqlc.arg(author_id))
  )
  AND (
      sqlc.arg(assignee_id)::bigint = 0
      OR (
          sqlc.arg(assignee_id) < 0
          AND NOT EXISTS (SELECT 1 FROM kanban_card_assignee ca WHERE ca.card_id = c.id)
      )
      OR (
          sqlc.arg(assignee_id) > 0
          AND EXISTS (
              SELECT 1 FROM kanban_card_assignee ca
              WHERE ca.card_id = c.id AND ca.user_id = sqlc.arg(assignee_id)
          )
      )
  )
  AND (
      sqlc.arg(completed)::text = ''
      OR (c.completed_at IS NOT NULL) = (sqlc.arg(completed) = 'true')
  )
  AND (
      sqlc.arg(archived)::text = ''
      OR (
          CASE WHEN c.parent_id IS NULL THEN c.is_archived ELSE COALESCE(parent.is_archived, FALSE) END
      ) = (sqlc.arg(archived) = 'true')
  )
  AND (
      sqlc.arg(kind)::text = ''
      OR (sqlc.arg(kind) = 'task' AND c.parent_id IS NULL)
      OR (sqlc.arg(kind) = 'subtask' AND c.parent_id IS NOT NULL)
  )
  AND (sqlc.narg('due_from')::timestamptz IS NULL OR c.due_date >= sqlc.narg('due_from'))
  AND (sqlc.narg('due_to')::timestamptz IS NULL OR c.due_date < sqlc.narg('due_to'))
  AND (sqlc.narg('created_from')::timestamptz IS NULL OR c.created_at >= sqlc.narg('created_from'))
  AND (sqlc.narg('created_to')::timestamptz IS NULL OR c.created_at < sqlc.narg('created_to'))
  AND (sqlc.narg('completed_from')::timestamptz IS NULL OR c.completed_at >= sqlc.narg('completed_from'))
  AND (sqlc.narg('completed_to')::timestamptz IS NULL OR c.completed_at < sqlc.narg('completed_to'))
  AND (
      sqlc.narg('archived_from')::timestamptz IS NULL
      OR (CASE WHEN c.parent_id IS NULL THEN c.archived_at ELSE parent.archived_at END) >= sqlc.narg('archived_from')
  )
  AND (
      sqlc.narg('archived_to')::timestamptz IS NULL
      OR (CASE WHEN c.parent_id IS NULL THEN c.archived_at ELSE parent.archived_at END) < sqlc.narg('archived_to')
  )
ORDER BY
  CASE WHEN sqlc.arg(sort)::text = 'title' AND sqlc.arg(sort_desc)::bool THEN c.title END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'title' AND NOT sqlc.arg(sort_desc)::bool THEN c.title END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'createdAt' AND sqlc.arg(sort_desc)::bool THEN c.created_at END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'createdAt' AND NOT sqlc.arg(sort_desc)::bool THEN c.created_at END ASC,
  CASE WHEN sqlc.arg(sort)::text = 'completedAt' AND sqlc.arg(sort_desc)::bool THEN c.completed_at END DESC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'completedAt' AND NOT sqlc.arg(sort_desc)::bool THEN c.completed_at END ASC NULLS LAST,
  CASE WHEN sqlc.arg(sort)::text = 'priority' AND sqlc.arg(sort_desc)::bool THEN
    CASE c.priority WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END
  END DESC,
  CASE WHEN sqlc.arg(sort)::text = 'priority' AND NOT sqlc.arg(sort_desc)::bool THEN
    CASE c.priority WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END
  END ASC,
  c.id ASC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- ==============================
-- TASK COLLABORANTS
-- ==============================

-- name: CountTaskCollaborants :one
WITH visible AS (
    SELECT p.id, p.owner_id
    FROM kanban_project p
    WHERE p.deleted_at IS NULL
      AND (
          p.owner_id = sqlc.arg(viewer_id)
          OR EXISTS (
              SELECT 1 FROM kanban_project_user pu
              WHERE pu.kanban_project_id = p.id
                AND pu.user_id = sqlc.arg(viewer_id)
          )
      )
),
ids AS (
    SELECT visible.owner_id AS user_id FROM visible
    UNION
    SELECT pu.user_id
    FROM kanban_project_user pu
    INNER JOIN visible ON visible.id = pu.kanban_project_id
)
SELECT COUNT(*)::bigint AS count
FROM ids
INNER JOIN users u ON u.id = ids.user_id
WHERE u.deleted_at IS NULL
  AND u.id <> sqlc.arg(viewer_id)
  AND (
      sqlc.arg(name_query)::text = ''
      OR u.lastname ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
      OR u.firstname ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
      OR u.login ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
      OR COALESCE(u.patronymic, '') ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
  );

-- name: ListTaskCollaborants :many
WITH visible AS (
    SELECT p.id, p.owner_id
    FROM kanban_project p
    WHERE p.deleted_at IS NULL
      AND (
          p.owner_id = sqlc.arg(viewer_id)
          OR EXISTS (
              SELECT 1 FROM kanban_project_user pu
              WHERE pu.kanban_project_id = p.id
                AND pu.user_id = sqlc.arg(viewer_id)
          )
      )
),
ids AS (
    SELECT visible.owner_id AS user_id FROM visible
    UNION
    SELECT pu.user_id
    FROM kanban_project_user pu
    INNER JOIN visible ON visible.id = pu.kanban_project_id
)
SELECT u.id, u.login, u.lastname, u.firstname, u.patronymic, u.avatar_name
FROM ids
INNER JOIN users u ON u.id = ids.user_id
WHERE u.deleted_at IS NULL
  AND u.id <> sqlc.arg(viewer_id)
  AND (
      sqlc.arg(name_query)::text = ''
      OR u.lastname ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
      OR u.firstname ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
      OR u.login ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
      OR COALESCE(u.patronymic, '') ILIKE '%' || sqlc.arg(name_query) || '%' ESCAPE '\'
  )
ORDER BY u.lastname ASC, u.firstname ASC, u.id ASC
LIMIT sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- ==============================
-- PROJECT HISTORY
-- ==============================

-- name: CreateProjectHistoryEntry :one
INSERT INTO kanban_project_history (
    project_id, user_id, action, entity_title, entity_link,
    entity_type, entity_id, card_id, payload
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING *;

-- name: ListProjectHistory :many
SELECT
    j.id,
    j.project_id,
    j.user_id,
    j.action,
    j.entity_title,
    j.entity_link,
    j.payload,
    j.created_at,
    (j.entity_type = 'card' AND j.card_id IS NOT NULL AND j.entity_id <> j.card_id) AS is_child,
    u.lastname,
    u.firstname,
    u.patronymic
FROM kanban_project_history j
LEFT JOIN users u ON u.id = j.user_id
WHERE j.project_id = sqlc.arg(project_id)
  AND (sqlc.arg(card_id)::bigint = 0 OR j.card_id = sqlc.arg(card_id))
  AND (sqlc.arg(user_id)::bigint = 0 OR j.user_id = sqlc.arg(user_id))
  AND (sqlc.arg(title_query)::text = '' OR j.entity_title ILIKE '%' || sqlc.arg(title_query) || '%' ESCAPE '\')
  AND (sqlc.arg(cursor)::bigint = 0 OR j.id < sqlc.arg(cursor))
ORDER BY j.id DESC
LIMIT sqlc.arg(page_limit);

-- name: GetProjectHistoryEntry :one
SELECT * FROM kanban_project_history
WHERE id = $1 LIMIT 1;

-- name: GetLastProjectHistoryByUser :one
SELECT * FROM kanban_project_history
WHERE project_id = sqlc.arg(project_id) AND user_id = sqlc.arg(user_id)
ORDER BY id DESC
LIMIT 1;

-- name: HasForeignNewerHistoryOverlap :one
SELECT EXISTS(
    SELECT 1 FROM kanban_project_history
    WHERE project_id = sqlc.arg(project_id)
      AND id > sqlc.arg(after_id)
      AND user_id IS DISTINCT FROM sqlc.arg(user_id)
      AND entity_type = sqlc.arg(entity_type)
      AND entity_id = sqlc.arg(entity_id)
);

-- name: DeleteProjectHistoryEntry :exec
DELETE FROM kanban_project_history
WHERE id = $1;

-- name: RestoreProject :exec
UPDATE kanban_project
SET deleted_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;

-- name: RestoreBoard :exec
UPDATE kanban_board
SET deleted_at = NULL, updated_at = CURRENT_TIMESTAMP
WHERE id = $1;
