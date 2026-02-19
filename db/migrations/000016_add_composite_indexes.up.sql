-- Migration 000016: Add composite indexes for query performance
-- Identified by database skills evaluation (DB optimizer patterns)

-- ── Composite index for FindSuccessfulPaidPurchaseByCustomer ─────────────────────────────────
-- Query: WHERE customer_id = $1 AND status = 'paid' ORDER BY paid_at DESC LIMIT 1
-- Enables index-only scan for this hot path (referral check, auto-renew pricing).
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_customer_status_paid
    ON purchase(customer_id, status, paid_at DESC);

-- ── Composite index for FindByInvoiceTypeAndStatus ───────────────────────────────────────────
-- Query: WHERE invoice_type = $1 AND status = $2
-- Used by crypto invoice checker cron every 5 seconds.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_invoice_status
    ON purchase(invoice_type, status);

-- ── Composite index for wallet transaction history ───────────────────────────────────────────
-- Query: WHERE customer_id = $1 ORDER BY created_at DESC LIMIT n
-- The standalone idx_wallet_transaction_customer exists but doesn't help with ORDER BY.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_wallet_tx_customer_created
    ON wallet_transaction(customer_id, created_at DESC);

-- ── Partial index for revenue view queries ───────────────────────────────────────────────────
-- Both revenue views filter: WHERE status = 'paid' AND paid_at IS NOT NULL
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_purchase_status_paid_at
    ON purchase(status, paid_at) WHERE status = 'paid';
