-- +goose Up
ALTER TABLE users
    ADD COLUMN username      TEXT UNIQUE,
    ADD COLUMN password_hash TEXT,
    ADD COLUMN is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN last_login_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users
    DROP COLUMN username,
    DROP COLUMN password_hash,
    DROP COLUMN is_admin,
    DROP COLUMN updated_at,
    DROP COLUMN last_login_at;
