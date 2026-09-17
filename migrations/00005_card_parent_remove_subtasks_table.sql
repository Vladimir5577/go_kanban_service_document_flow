-- +goose Up

ALTER TABLE kanban_card
    ADD COLUMN parent_id BIGINT REFERENCES kanban_card(id);

ALTER TABLE kanban_card
    ALTER COLUMN column_id DROP NOT NULL;

ALTER TABLE kanban_card
    ADD CONSTRAINT kanban_card_parent_xor_column
    CHECK ((parent_id IS NULL) = (column_id IS NOT NULL));

ALTER TABLE kanban_card
    ADD CONSTRAINT kanban_card_parent_not_self
    CHECK (parent_id IS NULL OR parent_id <> id);

CREATE INDEX idx_kanban_card_parent_id ON kanban_card (parent_id)
    WHERE parent_id IS NOT NULL;

ALTER TABLE kanban_card ADD COLUMN _old_subtask_id BIGINT;

INSERT INTO kanban_card (
    title, position, parent_id, column_id, created_at, updated_at, deleted_at,
    completed_at, _old_subtask_id
)
SELECT
    s.title,
    s.position,
    s.card_id,
    NULL,
    NOW(),
    NOW(),
    s.deleted_at,
    CASE WHEN LOWER(s.status) = 'done' THEN NOW() END,
    s.id
FROM kanban_card_subtask s;

INSERT INTO kanban_card_assignee (card_id, user_id)
SELECT c.id, s.user_id
FROM kanban_card c
JOIN kanban_card_subtask s ON s.id = c._old_subtask_id
WHERE s.user_id IS NOT NULL
ON CONFLICT DO NOTHING;

ALTER TABLE kanban_card DROP COLUMN _old_subtask_id;

UPDATE kanban_card child
SET deleted_at = parent.deleted_at, updated_at = NOW()
FROM kanban_card parent
WHERE child.parent_id = parent.id
  AND parent.deleted_at IS NOT NULL
  AND child.deleted_at IS NULL;

DELETE FROM kanban_project_history WHERE action LIKE 'subtask.%';

DROP TABLE kanban_card_subtask;

-- +goose Down

CREATE TABLE kanban_card_subtask (
    id         BIGSERIAL PRIMARY KEY,
    title      VARCHAR(500) NOT NULL,
    status     VARCHAR(255) NOT NULL DEFAULT 'to_do',
    position   DOUBLE PRECISION NOT NULL DEFAULT 0,
    card_id    BIGINT NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    user_id    BIGINT,
    deleted_at TIMESTAMPTZ(0)
);
CREATE INDEX idx_kanban_card_subtask_card_id ON kanban_card_subtask (card_id);
CREATE INDEX idx_kanban_card_subtask_user_id ON kanban_card_subtask (user_id);
CREATE INDEX idx_kanban_card_subtask_deleted_at ON kanban_card_subtask (deleted_at);

DELETE FROM kanban_card WHERE parent_id IS NOT NULL;

DROP INDEX IF EXISTS idx_kanban_card_parent_id;
ALTER TABLE kanban_card DROP CONSTRAINT IF EXISTS kanban_card_parent_not_self;
ALTER TABLE kanban_card DROP CONSTRAINT IF EXISTS kanban_card_parent_xor_column;
ALTER TABLE kanban_card DROP COLUMN IF EXISTS parent_id;
ALTER TABLE kanban_card ALTER COLUMN column_id SET NOT NULL;
