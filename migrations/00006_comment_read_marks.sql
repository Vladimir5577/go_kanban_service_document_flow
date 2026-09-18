-- +goose Up

-- Отметки прочтения комментариев: строка на заход, а не на комментарий.
-- Знак только растёт, старые строки не переписываются — по ним потом
-- восстанавливается, когда именно человек увидел конкретный комментарий.
-- Первичный ключ обслуживает оба запроса: DISTINCT ON получает готовый
-- порядок, MAX() берётся обратным сканом. Отдельный индекс не нужен.
CREATE TABLE kanban_card_read (
    card_id          BIGINT NOT NULL REFERENCES kanban_card(id) ON DELETE CASCADE,
    user_id          BIGINT NOT NULL,
    up_to_comment_id BIGINT NOT NULL,
    read_at          TIMESTAMPTZ(0) NOT NULL DEFAULT NOW(),
    PRIMARY KEY (card_id, user_id, up_to_comment_id)
);

-- +goose Down

DROP TABLE IF EXISTS kanban_card_read;
