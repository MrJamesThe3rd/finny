-- +goose Up
ALTER TABLE transactions
DROP CONSTRAINT chk_transactions_status;

ALTER TABLE transactions
ADD CONSTRAINT chk_transactions_status CHECK (status IN ('draft', 'pending_invoice', 'complete', 'no_invoice'));

-- +goose Down
ALTER TABLE transactions
DROP CONSTRAINT chk_transactions_status;

ALTER TABLE transactions
ADD CONSTRAINT chk_transactions_status CHECK (status IN ('draft', 'pending_invoice', 'complete'));
