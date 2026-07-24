-- +goose Up
-- Длинные имена файлов (>255 символов, напр. развёрнутые названия документов)
-- упирались в VARCHAR(255) и падали с 22001. Расширяем до 512;
-- в приложении стоит guard CodeFilenameTooLong на этот же лимит.
ALTER TABLE kanban_attachment ALTER COLUMN filename TYPE VARCHAR(512);

-- +goose Down
ALTER TABLE kanban_attachment ALTER COLUMN filename TYPE VARCHAR(255);
