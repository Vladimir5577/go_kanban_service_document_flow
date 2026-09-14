-- +goose Up

ALTER TABLE kanban_card
    ADD COLUMN deleted_at TIMESTAMPTZ(0);
CREATE INDEX idx_kanban_card_deleted_at ON kanban_card (deleted_at);

ALTER TABLE kanban_attachment
    ADD COLUMN deleted_at TIMESTAMPTZ(0);
CREATE INDEX idx_kanban_attachment_deleted_at ON kanban_attachment (deleted_at);

ALTER TABLE kanban_column
    ADD COLUMN deleted_at TIMESTAMPTZ(0);
CREATE INDEX idx_kanban_column_deleted_at ON kanban_column (deleted_at);

ALTER TABLE kanban_label
    ADD COLUMN deleted_at TIMESTAMPTZ(0);
CREATE INDEX idx_kanban_label_deleted_at ON kanban_label (deleted_at);

ALTER TABLE kanban_card_comment
    ADD COLUMN deleted_at TIMESTAMPTZ(0);
CREATE INDEX idx_kanban_card_comment_deleted_at ON kanban_card_comment (deleted_at);

ALTER TABLE kanban_card_subtask
    ADD COLUMN deleted_at TIMESTAMPTZ(0);
CREATE INDEX idx_kanban_card_subtask_deleted_at ON kanban_card_subtask (deleted_at);

-- История жестов проекта. Soft-delete проекта строки не трогает.
CREATE TABLE kanban_project_history (
    id           BIGSERIAL PRIMARY KEY,
    project_id   BIGINT NOT NULL REFERENCES kanban_project(id),
    user_id      BIGINT,
    action       VARCHAR(64) NOT NULL,
    entity_type  VARCHAR(32) NOT NULL,
    entity_id    BIGINT NOT NULL,
    -- Список карточки: card.*, comment.*, subtask.*, attachment.*, label.added/removed.
    -- NULL: колонка/доска/проект/участники, label.deleted (метка целиком, не карточка).
    card_id      BIGINT,
    entity_title TEXT NOT NULL DEFAULT '',
    entity_link  TEXT NOT NULL DEFAULT '',
    payload      JSONB NOT NULL,
    created_at   TIMESTAMPTZ(0) NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_project_history_project_created ON kanban_project_history (project_id, id DESC);
CREATE INDEX idx_project_history_project_user ON kanban_project_history (project_id, user_id, id DESC);
CREATE INDEX idx_project_history_entity ON kanban_project_history (project_id, entity_type, entity_id, id DESC);
CREATE INDEX idx_project_history_card ON kanban_project_history (card_id, id DESC) WHERE card_id IS NOT NULL;

-- Старые activity: без снапшота, undo нет (payload null/null).
INSERT INTO kanban_project_history (
    project_id, user_id, action, entity_type, entity_id, card_id,
    entity_title, entity_link, payload, created_at
)
SELECT
    b.kanban_project_id,
    a.user_id,
    CASE a.type
        WHEN 'created' THEN 'card.created'
        WHEN 'renamed' THEN 'card.updated.renamed'
        WHEN 'description_changed' THEN 'card.updated.description'
        WHEN 'priority_changed' THEN 'card.updated.priority'
        WHEN 'due_date_changed' THEN 'card.updated.due_date'
        WHEN 'color_changed' THEN 'card.updated.color'
        WHEN 'moved' THEN 'card.moved'
        WHEN 'assignee_added' THEN 'card.assignee_added'
        WHEN 'assignee_removed' THEN 'card.assignee_removed'
        WHEN 'label_added' THEN 'label.added'
        WHEN 'label_removed' THEN 'label.removed'
        WHEN 'comment_added' THEN 'comment.created'
        WHEN 'attachment_added' THEN 'attachment.created'
        WHEN 'attachment_removed' THEN 'attachment.deleted'
        WHEN 'subtask_added' THEN 'subtask.created'
        WHEN 'subtask_completed' THEN 'subtask.completed'
        WHEN 'subtask_reopened' THEN 'subtask.reopened'
        WHEN 'subtask_removed' THEN 'subtask.deleted'
        WHEN 'subtask_assigned' THEN 'subtask.assigned'
        WHEN 'subtask_unassigned' THEN 'subtask.unassigned'
        WHEN 'archived' THEN 'card.archived'
        WHEN 'restored' THEN 'card.restored'
        WHEN 'completed' THEN 'card.completed'
        WHEN 'reopened' THEN 'card.reopened'
        ELSE 'card.updated'
    END,
    'card',
    a.card_id,
    a.card_id,
    CASE
        WHEN a.type IN ('label_added', 'label_removed') THEN
            COALESCE(NULLIF(a.new_value, ''), NULLIF(a.old_value, ''), '')
        ELSE c.title
    END,
    CASE
        WHEN a.type = 'archived' THEN
            '/projects/' || b.kanban_project_id::text || '/board-' || b.id::text || '/archive'
        ELSE
            '/projects/' || b.kanban_project_id::text || '/board-' || b.id::text || '/task-' || a.card_id::text
    END,
    '{"before":null,"after":null}'::jsonb,
    a.created_at
FROM kanban_card_activity a
JOIN kanban_card c ON c.id = a.card_id
JOIN kanban_column col ON col.id = c.column_id
JOIN kanban_board b ON b.id = col.board_id
ORDER BY a.created_at, a.id;

DROP TABLE IF EXISTS kanban_card_activity;

-- +goose Down

CREATE TABLE kanban_card_activity (
    id         BIGSERIAL PRIMARY KEY,
    card_id    BIGINT NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    user_id    BIGINT,
    type       VARCHAR(40) NOT NULL,
    old_value  TEXT,
    new_value  TEXT,
    created_at TIMESTAMPTZ(0) NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_kanban_card_activity_card_id ON kanban_card_activity (card_id);
CREATE INDEX idx_card_activity_card_created ON kanban_card_activity (card_id, created_at);
CREATE INDEX idx_kanban_card_activity_user_id ON kanban_card_activity (user_id);

DROP TABLE IF EXISTS kanban_project_history;
DROP INDEX IF EXISTS idx_kanban_card_subtask_deleted_at;
ALTER TABLE kanban_card_subtask DROP COLUMN IF EXISTS deleted_at;
DROP INDEX IF EXISTS idx_kanban_card_comment_deleted_at;
ALTER TABLE kanban_card_comment DROP COLUMN IF EXISTS deleted_at;
DROP INDEX IF EXISTS idx_kanban_label_deleted_at;
ALTER TABLE kanban_label DROP COLUMN IF EXISTS deleted_at;
DROP INDEX IF EXISTS idx_kanban_column_deleted_at;
ALTER TABLE kanban_column DROP COLUMN IF EXISTS deleted_at;
DROP INDEX IF EXISTS idx_kanban_attachment_deleted_at;
ALTER TABLE kanban_attachment DROP COLUMN IF EXISTS deleted_at;
DROP INDEX IF EXISTS idx_kanban_card_deleted_at;
ALTER TABLE kanban_card DROP COLUMN IF EXISTS deleted_at;
