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

-- name: GetChildStats :one
-- Счётчик и хвостовая позиция для создания подзадачи: всё, что нужно,
-- без вычитывания самих детей и их исполнителей.
SELECT COUNT(*)::bigint AS total,
       COALESCE(MAX(position), 0)::double precision AS max_position
FROM kanban_card
WHERE parent_id = $1 AND deleted_at IS NULL;

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

-- name: UpdateCardPosition :one
-- Перестановка не должна затирать заголовок, который в этот же момент правит
-- кто-то другой, — поэтому узкий UPDATE, а не перезапись всей строки.
-- Старая позиция нужна истории как Before, и она же приезжает из FROM —
-- отдельное чтение карточки ради одного числа не требуется.
UPDATE kanban_card c
SET position = $2, updated_at = CURRENT_TIMESTAMP
FROM kanban_card old
WHERE c.id = $1 AND old.id = c.id AND c.deleted_at IS NULL
RETURNING c.id, c.title, c.position, c.updated_at, old.position AS old_position;

-- name: DeleteCard :exec
UPDATE kanban_card
SET deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
WHERE deleted_at IS NULL AND (id = $1 OR parent_id = $1);

-- name: RestoreCard :exec
-- Возвращаем только тех детей, которых удалили вместе с родителем: DeleteCard
-- ставит им один и тот же CURRENT_TIMESTAMP (время транзакции). Дети, удалённые
-- раньше и отдельно, к этой отмене отношения не имеют и остаются удалёнными.
-- Сторона FROM видит снимок до UPDATE, то есть p.deleted_at читается старым.
UPDATE kanban_card c
SET deleted_at = NULL, updated_at = CURRENT_TIMESTAMP
FROM kanban_card p
WHERE p.id = $1
  AND (c.id = p.id OR (c.parent_id = p.id AND c.deleted_at = p.deleted_at));

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
-- COMMENT READ MARKS
-- ==============================

-- Проверка «знак вырос» живёт внутри запроса: иначе между чтением максимума
-- и вставкой влезает соседняя вкладка того же пользователя. Устаревшее или
-- меньшее число просто не вставится, ON CONFLICT добивает точные дубли.
-- EXISTS обязателен: без него клиент присылает id из будущего и помечает
-- прочитанными комментарии, которых ещё нет.
-- name: MarkCommentsRead :exec
INSERT INTO kanban_card_read (card_id, user_id, up_to_comment_id)
SELECT sqlc.arg(card_id)::bigint, sqlc.arg(user_id)::bigint, sqlc.arg(up_to_comment_id)::bigint
WHERE EXISTS (
    SELECT 1 FROM kanban_card_comment
    WHERE id = sqlc.arg(up_to_comment_id)
      AND card_id = sqlc.arg(card_id)
      AND deleted_at IS NULL
)
AND sqlc.arg(up_to_comment_id) > COALESCE((
    SELECT MAX(up_to_comment_id) FROM kanban_card_read
    WHERE card_id = sqlc.arg(card_id) AND user_id = sqlc.arg(user_id)
), 0)
ON CONFLICT DO NOTHING;

-- Самый ранний заход, накрывший комментарий, — это и есть момент, когда
-- человек его увидел. Порядок берётся из первичного ключа, без сортировки.
-- name: GetCommentReaders :many
SELECT DISTINCT ON (user_id) user_id, read_at
FROM kanban_card_read
WHERE card_id = $1 AND up_to_comment_id >= $2
ORDER BY user_id, up_to_comment_id;

-- name: GetCardReadMark :one
SELECT COALESCE(MAX(up_to_comment_id), 0)::bigint AS last_read_comment_id
FROM kanban_card_read
WHERE card_id = $1 AND user_id = $2;


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


-- Список задач (ListTasks) собирается динамически в card_repository.go:
-- фильтров полтора десятка, и статический запрос с `$n = '' OR ...` не давал
-- планировщику взять индекс ни по одному из них.

-- ==============================
-- TASK COLLABORANTS
-- ==============================

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
SELECT COUNT(*) OVER ()::bigint AS total_count,
       u.id, u.login, u.lastname, u.firstname, u.patronymic, u.avatar_name
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
-- include_nested: откат *.created. Чужая история внутри контейнера тоже пересечение.
-- Карточка: card_id этой карточки или её детей, и сами дети (parent_id).
-- Колонка / доска: чужие записи карточек, колонок и меток внутри.
-- Проект: любая чужая запись проекта.
SELECT EXISTS(
    SELECT 1 FROM kanban_project_history h
    WHERE h.project_id = sqlc.arg(project_id)
      AND h.id > sqlc.arg(after_id)
      AND h.user_id IS DISTINCT FROM sqlc.arg(user_id)
      AND (
        (h.entity_type = sqlc.arg(entity_type) AND h.entity_id = sqlc.arg(entity_id))
        OR (
          sqlc.arg(include_nested)::bool
          AND CASE sqlc.arg(entity_type)
            WHEN 'card' THEN
              h.card_id = sqlc.arg(entity_id)
              OR h.card_id IN (SELECT id FROM kanban_card WHERE parent_id = sqlc.arg(entity_id))
              OR (h.entity_type = 'card' AND h.entity_id IN (SELECT id FROM kanban_card WHERE parent_id = sqlc.arg(entity_id)))
            WHEN 'column' THEN
              h.card_id IN (
                SELECT id FROM kanban_card
                WHERE column_id = sqlc.arg(entity_id)
                   OR parent_id IN (SELECT id FROM kanban_card WHERE column_id = sqlc.arg(entity_id))
              )
              OR (
                h.entity_type = 'card'
                AND h.entity_id IN (
                  SELECT id FROM kanban_card
                  WHERE column_id = sqlc.arg(entity_id)
                     OR parent_id IN (SELECT id FROM kanban_card WHERE column_id = sqlc.arg(entity_id))
                )
              )
            WHEN 'board' THEN
              (h.entity_type = 'column' AND h.entity_id IN (SELECT id FROM kanban_column WHERE board_id = sqlc.arg(entity_id)))
              OR (h.entity_type = 'label' AND h.entity_id IN (SELECT id FROM kanban_label WHERE board_id = sqlc.arg(entity_id)))
              OR h.card_id IN (
                SELECT c.id FROM kanban_card c
                WHERE c.column_id IN (SELECT id FROM kanban_column WHERE board_id = sqlc.arg(entity_id))
                   OR c.parent_id IN (
                     SELECT p.id FROM kanban_card p
                     WHERE p.column_id IN (SELECT id FROM kanban_column WHERE board_id = sqlc.arg(entity_id))
                   )
              )
              OR (
                h.entity_type = 'card'
                AND h.entity_id IN (
                  SELECT c.id FROM kanban_card c
                  WHERE c.column_id IN (SELECT id FROM kanban_column WHERE board_id = sqlc.arg(entity_id))
                     OR c.parent_id IN (
                       SELECT p.id FROM kanban_card p
                       WHERE p.column_id IN (SELECT id FROM kanban_column WHERE board_id = sqlc.arg(entity_id))
                     )
                )
              )
            WHEN 'project' THEN TRUE
            ELSE FALSE
          END
        )
      )
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
