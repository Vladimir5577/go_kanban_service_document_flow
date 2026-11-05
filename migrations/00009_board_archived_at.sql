-- +goose Up
-- Архив доски: NULL — активна, время — когда убрали из вкладок. Колонки и задачи остаются.
ALTER TABLE kanban_board
    ADD COLUMN archived_at TIMESTAMPTZ(0);

-- +goose Down
ALTER TABLE kanban_board DROP COLUMN archived_at;
