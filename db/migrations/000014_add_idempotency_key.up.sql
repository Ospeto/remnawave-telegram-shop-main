ALTER TABLE purchase ADD COLUMN idempotency_key UUID;
CREATE UNIQUE INDEX idx_purchase_idempotency_key ON purchase(idempotency_key) WHERE idempotency_key IS NOT NULL;
