-- +goose Up
-- Колонка «сделано»: при отметке задачи выполненной фронт переносит карточку сюда.
ALTER TABLE kanban_board
    ADD COLUMN done_column_id BIGINT REFERENCES kanban_column(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE kanban_board DROP COLUMN done_column_id;
