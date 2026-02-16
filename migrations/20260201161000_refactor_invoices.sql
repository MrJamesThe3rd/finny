-- +goose Up
CREATE TABLE invoices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    url TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE transactions ADD COLUMN invoice_id UUID REFERENCES invoices(id);

INSERT INTO invoices (url)
SELECT DISTINCT invoice_url FROM transactions WHERE invoice_url IS NOT NULL;

UPDATE transactions t
SET invoice_id = i.id
FROM invoices i
WHERE t.invoice_url = i.url;

ALTER TABLE transactions DROP COLUMN invoice_url;

-- +goose Down
ALTER TABLE transactions ADD COLUMN invoice_url TEXT;

UPDATE transactions t
SET invoice_url = i.url
FROM invoices i
WHERE t.invoice_id = i.id;

ALTER TABLE transactions DROP COLUMN invoice_id;

DROP TABLE invoices;
