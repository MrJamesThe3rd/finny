-- +goose Up
ALTER TABLE transactions 
ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'complete',
ADD COLUMN raw_description TEXT;

ALTER TABLE transactions
ADD CONSTRAINT chk_transactions_status CHECK (status IN ('draft', 'pending_invoice', 'complete'));

CREATE TABLE description_mappings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    raw_pattern TEXT NOT NULL,
    preferred_description TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_transactions_status ON transactions(status);
CREATE INDEX idx_mappings_raw_pattern ON description_mappings(raw_pattern);

-- +goose Down
DROP TABLE description_mappings;

ALTER TABLE transactions
DROP CONSTRAINT chk_transactions_status,
DROP COLUMN raw_description,
DROP COLUMN status;
