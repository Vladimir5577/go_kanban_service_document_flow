-- +goose Up
CREATE SEQUENCE kanban_card_deletion_batch_seq;
CREATE TABLE kanban_card_deletion_batch (
    card_id BIGINT PRIMARY KEY REFERENCES kanban_card(id) ON DELETE CASCADE,
    batch_id BIGINT NOT NULL
);

-- +goose Down
DROP TABLE kanban_card_deletion_batch;
DROP SEQUENCE kanban_card_deletion_batch_seq;
