-- Rollback indexes
DROP INDEX IF EXISTS idx_purchase_customer_id;
DROP INDEX IF EXISTS idx_purchase_txn_id;
DROP INDEX IF EXISTS idx_customer_telegram_id_unique;

-- Restore old hash index (optional, but good for exact rollback)
CREATE INDEX IF NOT EXISTS idx_customer_telegram_id ON customer USING hash (telegram_id);
