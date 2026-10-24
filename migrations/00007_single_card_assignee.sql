-- +goose Up
-- Fails on existing duplicates; review them explicitly, never discard assignments.
ALTER TABLE kanban_card_assignee ADD CONSTRAINT uniq_card_assignee UNIQUE (card_id);

-- +goose Down
ALTER TABLE kanban_card_assignee DROP CONSTRAINT uniq_card_assignee;
