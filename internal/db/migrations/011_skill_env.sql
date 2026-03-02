-- +goose Up
ALTER TABLE skills ADD COLUMN requires_env TEXT NOT NULL DEFAULT '[]';

-- +goose Down
ALTER TABLE skills DROP COLUMN requires_env;
