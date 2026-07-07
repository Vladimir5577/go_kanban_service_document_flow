-- +goose Up

-- ============================================================================
-- НАЧАЛЬНАЯ СХЕМА БД КАНБАН-МИКРОСЕРВИСА
-- Имена таблиц совпадают с Doctrine (Symfony) для совместимости при миграции.
-- FK на users НЕ ставим: это реплика, синхронизируемая через RabbitMQ.
-- FK между канбан-таблицами — полноценные (Go владеет доменом).
-- ============================================================================

-- Реплика пользователей из Symfony (синхронизация через RabbitMQ)
CREATE TABLE users (
    id          INTEGER PRIMARY KEY,
    login       VARCHAR(50) NOT NULL UNIQUE,
    lastname    VARCHAR(50) NOT NULL,
    firstname   VARCHAR(50) NOT NULL,
    patronymic  VARCHAR(50),
    avatar_name VARCHAR(255),
    deleted_at  TIMESTAMP(0),
    synced_at   TIMESTAMP(0) NOT NULL DEFAULT NOW()
);

-- Проекты канбана
CREATE TABLE kanban_project (
    id            SERIAL PRIMARY KEY,
    name          VARCHAR(255) NOT NULL,
    description   TEXT,
    owner_id      INTEGER NOT NULL,
    created_by_id INTEGER,
    created_at    TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMP(0)
);
CREATE INDEX idx_kanban_project_owner_id ON kanban_project (owner_id);
CREATE INDEX idx_kanban_project_created_by_id ON kanban_project (created_by_id);

-- Папки проектов пользователей (группировка проектов в сайдбаре)
CREATE TABLE kanban_project_user_folder (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(255) NOT NULL,
    user_id    INTEGER NOT NULL,
    position   DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP(0) NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_kanban_project_user_folder_user_id ON kanban_project_user_folder (user_id);
CREATE INDEX idx_folder_user_position ON kanban_project_user_folder (user_id, position);

-- Участники проекта
CREATE TABLE kanban_project_user (
    id                SERIAL PRIMARY KEY,
    kanban_project_id INTEGER NOT NULL REFERENCES kanban_project(id) ON DELETE RESTRICT,
    user_id           INTEGER NOT NULL,
    role              VARCHAR(255) NOT NULL,
    folder_id         INTEGER REFERENCES kanban_project_user_folder(id) ON DELETE SET NULL,
    position          DOUBLE PRECISION NOT NULL DEFAULT 0,
    CONSTRAINT uniq_project_user UNIQUE (kanban_project_id, user_id)
);
CREATE INDEX idx_project_user_user_id ON kanban_project_user (user_id);
CREATE INDEX idx_project_user_folder_position ON kanban_project_user (user_id, folder_id, position);
CREATE INDEX idx_project_user_project_id ON kanban_project_user (kanban_project_id);
CREATE INDEX idx_project_user_folder_id ON kanban_project_user (folder_id);

-- Доски
CREATE TABLE kanban_board (
    id                SERIAL PRIMARY KEY,
    title             VARCHAR(200) NOT NULL,
    position          DOUBLE PRECISION NOT NULL DEFAULT 0,
    kanban_project_id INTEGER NOT NULL REFERENCES kanban_project(id) ON DELETE RESTRICT,
    created_by_id     INTEGER NOT NULL,
    created_at        TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMP(0)
);
CREATE INDEX idx_kanban_board_project_id ON kanban_board (kanban_project_id);
CREATE INDEX idx_kanban_board_created_by_id ON kanban_board (created_by_id);

-- Колонки
CREATE TABLE kanban_column (
    id           SERIAL PRIMARY KEY,
    title        VARCHAR(200) NOT NULL,
    header_color VARCHAR(30) NOT NULL DEFAULT 'bg-primary',
    position     DOUBLE PRECISION NOT NULL DEFAULT 0,
    board_id     INTEGER NOT NULL REFERENCES kanban_board(id) ON DELETE CASCADE
);
CREATE INDEX idx_kanban_column_board_id ON kanban_column (board_id);

-- Карточки (задачи)
CREATE TABLE kanban_card (
    id              SERIAL PRIMARY KEY,
    title           VARCHAR(500) NOT NULL,
    description     TEXT,
    position        DOUBLE PRECISION NOT NULL DEFAULT 0,
    due_date        TIMESTAMP(0),
    priority        VARCHAR(20),
    is_archived     BOOLEAN NOT NULL DEFAULT FALSE,
    archived_at     TIMESTAMP(0),
    archived_by_id  INTEGER,
    completed_at    TIMESTAMP(0),
    completed_by_id INTEGER,
    column_id       INTEGER NOT NULL REFERENCES kanban_column(id) ON DELETE RESTRICT,
    created_by_id   INTEGER,
    border_color    VARCHAR(20),
    created_at      TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP(0) NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_kanban_card_column_id ON kanban_card (column_id);
CREATE INDEX idx_kanban_card_created_by_id ON kanban_card (created_by_id);
CREATE INDEX idx_kanban_card_archived_by_id ON kanban_card (archived_by_id);
CREATE INDEX idx_kanban_card_completed_by_id ON kanban_card (completed_by_id);
CREATE INDEX idx_kanban_card_is_archived_archived_at ON kanban_card (is_archived, archived_at);

-- Метки (привязаны к доске)
CREATE TABLE kanban_label (
    id       SERIAL PRIMARY KEY,
    name     VARCHAR(100) NOT NULL,
    color    VARCHAR(30) NOT NULL,
    board_id INTEGER NOT NULL REFERENCES kanban_board(id) ON DELETE CASCADE
);
CREATE INDEX idx_kanban_label_board_id ON kanban_label (board_id);

-- Связь карточка <-> метка (M2M)
CREATE TABLE kanban_card_label (
    kanban_card_id  INTEGER NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    kanban_label_id INTEGER NOT NULL REFERENCES kanban_label(id) ON DELETE CASCADE,
    PRIMARY KEY (kanban_card_id, kanban_label_id)
);
CREATE INDEX idx_card_label_card_id ON kanban_card_label (kanban_card_id);
CREATE INDEX idx_card_label_label_id ON kanban_card_label (kanban_label_id);

-- Исполнители карточки (M2M)
CREATE TABLE kanban_card_assignee (
    card_id INTEGER NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL,
    PRIMARY KEY (card_id, user_id)
);
CREATE INDEX idx_kanban_card_assignee_card_id ON kanban_card_assignee (card_id);
CREATE INDEX idx_kanban_card_assignee_user_id ON kanban_card_assignee (user_id);

-- Подзадачи (чеклист)
CREATE TABLE kanban_card_subtask (
    id       SERIAL PRIMARY KEY,
    title    VARCHAR(500) NOT NULL,
    status   VARCHAR(255) NOT NULL DEFAULT 'to_do',
    position DOUBLE PRECISION NOT NULL DEFAULT 0,
    card_id  INTEGER NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    user_id  INTEGER
);
CREATE INDEX idx_kanban_card_subtask_card_id ON kanban_card_subtask (card_id);
CREATE INDEX idx_kanban_card_subtask_user_id ON kanban_card_subtask (user_id);

-- Комментарии (чат)
CREATE TABLE kanban_card_comment (
    id         SERIAL PRIMARY KEY,
    body       TEXT NOT NULL,
    card_id    INTEGER NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    author_id  INTEGER NOT NULL,
    created_at TIMESTAMP(0) NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP(0)
);
CREATE INDEX idx_kanban_card_comment_card_id ON kanban_card_comment (card_id);
CREATE INDEX idx_kanban_card_comment_author_id ON kanban_card_comment (author_id);

-- История действий
CREATE TABLE kanban_card_activity (
    id         SERIAL PRIMARY KEY,
    card_id    INTEGER NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    user_id    INTEGER,
    type       VARCHAR(40) NOT NULL,
    old_value  TEXT,
    new_value  TEXT,
    created_at TIMESTAMP(0) NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_kanban_card_activity_card_id ON kanban_card_activity (card_id);
CREATE INDEX idx_card_activity_card_created ON kanban_card_activity (card_id, created_at);
CREATE INDEX idx_kanban_card_activity_user_id ON kanban_card_activity (user_id);

-- Вложения
CREATE TABLE kanban_attachment (
    id           SERIAL PRIMARY KEY,
    filename     VARCHAR(255) NOT NULL,
    storage_key  VARCHAR(500) NOT NULL,
    content_type VARCHAR(100) NOT NULL,
    size_bytes   INTEGER NOT NULL,
    card_id      INTEGER NOT NULL REFERENCES kanban_card(id) ON DELETE RESTRICT,
    context      VARCHAR(20) NOT NULL DEFAULT 'info',
    author_id    INTEGER,
    created_at   TIMESTAMP(0) NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_kanban_attachment_card_id ON kanban_attachment (card_id);
CREATE INDEX idx_kanban_attachment_author_id ON kanban_attachment (author_id);
CREATE INDEX idx_kanban_attachment_card_id_context ON kanban_attachment (card_id, context);


-- +goose Down
DROP TABLE IF EXISTS kanban_attachment;
DROP TABLE IF EXISTS kanban_card_activity;
DROP TABLE IF EXISTS kanban_card_comment;
DROP TABLE IF EXISTS kanban_card_subtask;
DROP TABLE IF EXISTS kanban_card_assignee;
DROP TABLE IF EXISTS kanban_card_label;
DROP TABLE IF EXISTS kanban_label;
DROP TABLE IF EXISTS kanban_card;
DROP TABLE IF EXISTS kanban_column;
DROP TABLE IF EXISTS kanban_board;
DROP TABLE IF EXISTS kanban_project_user;
DROP TABLE IF EXISTS kanban_project_user_folder;
DROP TABLE IF EXISTS kanban_project;
DROP TABLE IF EXISTS users;
