-- +goose Up

-- User-configured storage backends
CREATE TABLE document_backends (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    type       TEXT NOT NULL,
    name       TEXT NOT NULL,
    config     JSONB NOT NULL DEFAULT '{}',
    enabled    BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_document_backends_user_id ON document_backends(user_id);

-- Logical documents (metadata only; content lives on backends)
CREATE TABLE documents (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    filename   TEXT NOT NULL,
    mime_type  TEXT NOT NULL DEFAULT 'application/octet-stream',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_documents_user_id ON documents(user_id);

-- Per-backend storage locations for each document
CREATE TABLE document_locations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    backend_id  UUID NOT NULL REFERENCES document_backends(id),
    key         TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(document_id, backend_id)
);

CREATE INDEX idx_document_locations_document_id ON document_locations(document_id);

-- Seed the legacy Paperless backend for the default user.
-- Config (base_url + token) is populated at application startup from env vars.
INSERT INTO document_backends (id, user_id, type, name, config, enabled)
VALUES (
    '00000000-0000-0000-0000-000000000002',
    '00000000-0000-0000-0000-000000000001',
    'paperless',
    'Paperless (migrated)',
    '{}',
    true
);

-- Migrate existing invoices → documents + locations.

-- 1. Create a document record for each invoice (preserving id so invoice_id refs still work).
INSERT INTO documents (id, user_id, filename, mime_type, created_at)
SELECT id, user_id, 'invoice', 'application/octet-stream', created_at
FROM invoices;

-- 2. Create a location for each Paperless-format invoice URL, extracting the numeric key.
INSERT INTO document_locations (document_id, backend_id, key)
SELECT
    id,
    '00000000-0000-0000-0000-000000000002',
    (regexp_match(url, '/api/documents/(\d+)/'))[1]
FROM invoices
WHERE url LIKE '%/api/documents/%'
  AND (regexp_match(url, '/api/documents/(\d+)/'))[1] IS NOT NULL;

-- 3. Rename invoice_id → document_id on transactions (preserving existing refs).
ALTER TABLE transactions RENAME COLUMN invoice_id TO document_id;
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_invoice_id_fkey;
ALTER TABLE transactions
    ADD CONSTRAINT transactions_document_id_fkey
    FOREIGN KEY (document_id) REFERENCES documents(id);

-- 4. Drop the now-migrated invoices table.
DROP TABLE invoices;

-- +goose Down

-- Recreate invoices table (schema matches the 0A state).
CREATE TABLE invoices (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id),
    url        TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT invoices_user_url_unique UNIQUE (user_id, url)
);

-- Restore invoice rows from document_locations (URL is approximate).
INSERT INTO invoices (id, user_id, url, created_at)
SELECT
    dl.document_id,
    d.user_id,
    'https://unknown/api/documents/' || dl.key || '/download/',
    d.created_at
FROM document_locations dl
JOIN documents d ON dl.document_id = d.id;

-- Restore invoice_id on transactions.
ALTER TABLE transactions RENAME COLUMN document_id TO invoice_id;
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_document_id_fkey;
ALTER TABLE transactions
    ADD CONSTRAINT transactions_invoice_id_fkey
    FOREIGN KEY (invoice_id) REFERENCES invoices(id);

DROP TABLE document_locations;
DROP TABLE documents;
DROP TABLE document_backends;
