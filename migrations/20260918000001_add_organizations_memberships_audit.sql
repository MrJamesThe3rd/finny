-- +goose Up

-- Pre-flight guard. The backfill below collapses every existing user into one
-- organization and makes each an owner of it, and the last step then drops
-- user_id — unrecoverable. With one user that is a no-op; with two it silently
-- commingles two people's books. Fail loudly instead of assuming.
-- +goose StatementBegin
DO $$
DECLARE
    user_count BIGINT;
BEGIN
    SELECT count(*) INTO user_count FROM users;

    IF user_count <> 1 THEN
        RAISE EXCEPTION
            'org backfill expects exactly 1 user, found %. Migrate by hand.', user_count;
    END IF;
END
$$;
-- +goose StatementEnd

-- An organization is a set of books: a client company, or a personal account.
-- nif is nullable and NOT unique — a personal org has none, a non-PT client has
-- a different identifier, and a global UNIQUE would let one accountant squat a
-- client's NIF and would act as an enumeration oracle at POST /orgs.
CREATE TABLE organizations (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    nif        TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Append-only: revocation sets revoked_at, rows are never deleted, so the
-- record that someone had access during a period they signed off survives.
-- ON DELETE RESTRICT on user_id is deliberate — CASCADE would erase that
-- record and could strip an org of its last owner. auth.Service.DeleteUser
-- checks for memberships and returns 409 rather than hitting the FK.
CREATE TABLE memberships (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    role       TEXT NOT NULL CHECK (role IN ('owner', 'accountant')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (user_id, org_id)
);

CREATE INDEX idx_memberships_user_id ON memberships(user_id);
CREATE INDEX idx_memberships_org_id ON memberships(org_id);

INSERT INTO organizations (id, name, nif)
VALUES (
    '00000000-0000-0000-0000-000000000010',
    'VibrantGarden Unipessoal, Lda',
    '517948974'
);

INSERT INTO memberships (user_id, org_id, role)
SELECT id, '00000000-0000-0000-0000-000000000010', 'owner'
FROM users;

-- transactions
ALTER TABLE transactions
    ADD COLUMN org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010'
    REFERENCES organizations(id);
ALTER TABLE transactions ALTER COLUMN org_id DROP DEFAULT;
CREATE INDEX idx_transactions_org_id ON transactions(org_id);

-- description_mappings
ALTER TABLE description_mappings
    ADD COLUMN org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010'
    REFERENCES organizations(id);
ALTER TABLE description_mappings ALTER COLUMN org_id DROP DEFAULT;
CREATE INDEX idx_description_mappings_org_id ON description_mappings(org_id);

-- document_backends
ALTER TABLE document_backends
    ADD COLUMN org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010'
    REFERENCES organizations(id);
ALTER TABLE document_backends ALTER COLUMN org_id DROP DEFAULT;
CREATE INDEX idx_document_backends_org_id ON document_backends(org_id);

-- documents
ALTER TABLE documents
    ADD COLUMN org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000010'
    REFERENCES organizations(id);
ALTER TABLE documents ALTER COLUMN org_id DROP DEFAULT;
CREATE INDEX idx_documents_org_id ON documents(org_id);

-- Collapsing users into one org can collide on (org_id, raw_pattern), and
-- ADD CONSTRAINT would abort the migration. Mirrors 20260404000001:36-41.
-- The pre-flight guard above makes this a no-op in practice.
DELETE FROM description_mappings
WHERE id NOT IN (
    SELECT DISTINCT ON (org_id, raw_pattern) id
    FROM description_mappings
    ORDER BY org_id, raw_pattern, created_at DESC
);

ALTER TABLE description_mappings
    DROP CONSTRAINT description_mappings_user_pattern_unique;
ALTER TABLE description_mappings
    ADD CONSTRAINT description_mappings_org_pattern_unique UNIQUE (org_id, raw_pattern);

-- Attribution now lives in audit_log, which also covers mutation.
ALTER TABLE transactions DROP COLUMN user_id;
ALTER TABLE description_mappings DROP COLUMN user_id;
ALTER TABLE document_backends DROP COLUMN user_id;
ALTER TABLE documents DROP COLUMN user_id;

-- Append-only. Every state change writes its row in the same transaction as
-- the change itself. old_value/new_value are opaque JSONB of the changed
-- fields — no VAT, no categories, no derived accounting state.
-- actor_user_id carries no FK on purpose: the log outlives the actor, and
-- memberships already block deleting a user who has acted.
CREATE TABLE audit_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id),
    actor_user_id UUID NOT NULL,
    subject_type  TEXT NOT NULL,
    subject_id    UUID NOT NULL,
    action        TEXT NOT NULL,
    old_value     JSONB,
    new_value     JSONB,
    at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_org_at ON audit_log(org_id, at DESC);
CREATE INDEX idx_audit_log_subject ON audit_log(subject_type, subject_id);

-- +goose Down

DROP TABLE audit_log;

-- Restores the column, not the data: attribution written after the up
-- migration lives in audit_log and cannot be projected back.
ALTER TABLE documents
    ADD COLUMN user_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'
    REFERENCES users(id);
ALTER TABLE documents ALTER COLUMN user_id DROP DEFAULT;

ALTER TABLE document_backends
    ADD COLUMN user_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'
    REFERENCES users(id);
ALTER TABLE document_backends ALTER COLUMN user_id DROP DEFAULT;

ALTER TABLE description_mappings
    ADD COLUMN user_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'
    REFERENCES users(id);
ALTER TABLE description_mappings ALTER COLUMN user_id DROP DEFAULT;

ALTER TABLE transactions
    ADD COLUMN user_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'
    REFERENCES users(id);
ALTER TABLE transactions ALTER COLUMN user_id DROP DEFAULT;

ALTER TABLE description_mappings
    DROP CONSTRAINT description_mappings_org_pattern_unique;
ALTER TABLE description_mappings
    ADD CONSTRAINT description_mappings_user_pattern_unique UNIQUE (user_id, raw_pattern);

DROP INDEX idx_documents_org_id;
ALTER TABLE documents DROP COLUMN org_id;

DROP INDEX idx_document_backends_org_id;
ALTER TABLE document_backends DROP COLUMN org_id;

DROP INDEX idx_description_mappings_org_id;
ALTER TABLE description_mappings DROP COLUMN org_id;

DROP INDEX idx_transactions_org_id;
ALTER TABLE transactions DROP COLUMN org_id;

DROP TABLE memberships;
DROP TABLE organizations;
