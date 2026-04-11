-- +goose Up

CREATE TABLE users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO users (id, email, name)
VALUES ('00000000-0000-0000-0000-000000000001', 'default@finny.local', 'Default User');

-- transactions
ALTER TABLE transactions
    ADD COLUMN user_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'
    REFERENCES users(id);
ALTER TABLE transactions ALTER COLUMN user_id DROP DEFAULT;
CREATE INDEX idx_transactions_user_id ON transactions(user_id);

-- invoices: add user_id and replace the single-column unique with a compound one
ALTER TABLE invoices
    ADD COLUMN user_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'
    REFERENCES users(id);
ALTER TABLE invoices ALTER COLUMN user_id DROP DEFAULT;
ALTER TABLE invoices DROP CONSTRAINT invoices_url_key;
ALTER TABLE invoices ADD CONSTRAINT invoices_user_url_unique UNIQUE (user_id, url);

-- description_mappings: add user_id and a unique constraint (enables upsert)
ALTER TABLE description_mappings
    ADD COLUMN user_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'
    REFERENCES users(id);
ALTER TABLE description_mappings ALTER COLUMN user_id DROP DEFAULT;

-- Remove duplicate (user_id, raw_pattern) rows that exist before the unique
-- constraint is added, keeping only the most recently created mapping.
DELETE FROM description_mappings
WHERE id NOT IN (
    SELECT DISTINCT ON (user_id, raw_pattern) id
    FROM description_mappings
    ORDER BY user_id, raw_pattern, created_at DESC
);

ALTER TABLE description_mappings
    ADD CONSTRAINT description_mappings_user_pattern_unique UNIQUE (user_id, raw_pattern);

-- +goose Down

ALTER TABLE description_mappings DROP CONSTRAINT description_mappings_user_pattern_unique;
ALTER TABLE description_mappings DROP COLUMN user_id;

ALTER TABLE invoices DROP CONSTRAINT invoices_user_url_unique;
ALTER TABLE invoices ADD CONSTRAINT invoices_url_key UNIQUE (url);
ALTER TABLE invoices DROP COLUMN user_id;

DROP INDEX idx_transactions_user_id;
ALTER TABLE transactions DROP COLUMN user_id;

DROP TABLE users;
