-- +goose Up
ALTER TABLE api_keys ADD COLUMN base_url TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN base_url;
