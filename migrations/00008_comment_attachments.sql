-- +goose Up

-- Файлы комментариев живут в своей таблице: те же поля, что у kanban_attachment,
-- только вместо card_id — comment_id, а автора нет: файл кладёт только автор
-- комментария, он уже в kanban_card_comment.author_id. Поле context у файлов
-- карточки уходит: раньше оно отличало «файл в чате» от «файла карточки».
CREATE TABLE kanban_card_comment_attachment (
    id           BIGSERIAL PRIMARY KEY,
    filename     VARCHAR(512) NOT NULL,
    storage_key  VARCHAR(500) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size_bytes   BIGINT NOT NULL,
    comment_id   BIGINT NOT NULL REFERENCES kanban_card_comment(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ(0) NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMPTZ(0)
);
CREATE INDEX idx_kanban_card_comment_attachment_comment_id ON kanban_card_comment_attachment (comment_id);
CREATE INDEX idx_kanban_card_comment_attachment_deleted_at ON kanban_card_comment_attachment (deleted_at);

-- Каждое живое chat-вложение становится комментарием без текста с одним
-- файлом. Временная колонка нужна, чтобы связать вставленный комментарий
-- с исходным вложением: RETURNING не отдаёт колонки источника.
ALTER TABLE kanban_card_comment ADD COLUMN migrated_attachment_id BIGINT;

INSERT INTO kanban_card_comment (body, card_id, author_id, created_at, migrated_attachment_id)
SELECT '', a.card_id, COALESCE(a.author_id, c.created_by_id), a.created_at, a.id
FROM kanban_attachment a
JOIN kanban_card c ON c.id = a.card_id
WHERE a.context IN ('chat', 'comment')
  AND a.deleted_at IS NULL
  AND COALESCE(a.author_id, c.created_by_id) IS NOT NULL
ORDER BY a.created_at, a.id;

INSERT INTO kanban_card_comment_attachment (filename, storage_key, content_type, size_bytes, comment_id, created_at)
SELECT a.filename, a.storage_key, a.content_type, a.size_bytes, c.id, a.created_at
FROM kanban_attachment a
JOIN kanban_card_comment c ON c.migrated_attachment_id = a.id
ORDER BY a.created_at, a.id;

-- Новые комментарии получили свежие id и без этого всплыли бы всем как
-- непрочитанные. Кто дочитал карточку до конца (отметка не ниже последнего
-- живого обычного комментария), получает отметку на последнем мигрированном.
INSERT INTO kanban_card_read (card_id, user_id, up_to_comment_id)
SELECT r.card_id, r.user_id, m.max_migrated
FROM (
    SELECT card_id, user_id, MAX(up_to_comment_id) AS mark
    FROM kanban_card_read
    GROUP BY card_id, user_id
) r
JOIN (
    SELECT card_id, MAX(id) AS max_migrated
    FROM kanban_card_comment
    WHERE migrated_attachment_id IS NOT NULL
    GROUP BY card_id
) m ON m.card_id = r.card_id
LEFT JOIN (
    SELECT card_id, MAX(id) AS max_real
    FROM kanban_card_comment
    WHERE migrated_attachment_id IS NULL AND deleted_at IS NULL
    GROUP BY card_id
) x ON x.card_id = r.card_id
WHERE r.mark >= COALESCE(x.max_real, 0)
ON CONFLICT DO NOTHING;

-- Перенесённые файлы уходят из таблицы карточки — иначе после DROP COLUMN
-- context они стали бы файлами карточки. Удалённые chat-файлы тоже убираем,
-- чтобы «Отменить» в истории не вернул их уже как файл карточки.
DELETE FROM kanban_attachment a
USING kanban_card_comment c
WHERE c.migrated_attachment_id = a.id;

DELETE FROM kanban_attachment
WHERE context IN ('chat', 'comment') AND deleted_at IS NOT NULL;

ALTER TABLE kanban_card_comment DROP COLUMN migrated_attachment_id;

DROP INDEX IF EXISTS idx_kanban_attachment_card_id_context;
ALTER TABLE kanban_attachment DROP COLUMN context;

-- +goose Down

-- Откат приблизительный: файлы комментариев снова становятся chat-файлами
-- карточки (с новыми id, автор — автор комментария), комментарии без текста,
-- у которых были только файлы, удаляются вместе со своими файлами (ON DELETE CASCADE).
ALTER TABLE kanban_attachment ADD COLUMN context VARCHAR(20) NOT NULL DEFAULT 'info';
CREATE INDEX idx_kanban_attachment_card_id_context ON kanban_attachment (card_id, context);

INSERT INTO kanban_attachment (filename, storage_key, content_type, size_bytes, card_id, context, author_id, created_at, deleted_at)
SELECT a.filename, a.storage_key, a.content_type, a.size_bytes, c.card_id, 'chat', c.author_id, a.created_at, a.deleted_at
FROM kanban_card_comment_attachment a
JOIN kanban_card_comment c ON c.id = a.comment_id
ORDER BY a.created_at, a.id;

DELETE FROM kanban_card_comment c
WHERE c.body = ''
  AND EXISTS (SELECT 1 FROM kanban_card_comment_attachment a WHERE a.comment_id = c.id);

DROP TABLE IF EXISTS kanban_card_comment_attachment;
