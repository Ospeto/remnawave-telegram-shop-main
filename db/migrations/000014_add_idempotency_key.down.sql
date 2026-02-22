DROP INDEX IF EXISTS idx_purchase_idempotency_key;
ALTER TABLE purchase DROP COLUMN IF EXISTS idempotency_key;
