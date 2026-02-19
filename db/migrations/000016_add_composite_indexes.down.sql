-- Rollback for migration 000016

DROP INDEX CONCURRENTLY IF EXISTS idx_purchase_customer_status_paid;
DROP INDEX CONCURRENTLY IF EXISTS idx_purchase_invoice_status;
DROP INDEX CONCURRENTLY IF EXISTS idx_wallet_tx_customer_created;
DROP INDEX CONCURRENTLY IF EXISTS idx_purchase_status_paid_at;
