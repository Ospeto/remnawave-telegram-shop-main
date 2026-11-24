-- Migration 000037: partial index for the latest awaiting-verification purchase lookup
-- Audit finding P5 (data/wavy-shop-audit-t1/report.md §4)

-- ── Index for buildLatestAwaitingVerificationByCustomerSelect ────────────────────────────────
-- Query (internal/database/purchase.go:224-236):
--   WHERE customer_id = $1 AND invoice_type IN ('mobile_banking','wallet_topup')
--     AND status IN ('pending','new') AND created_at >= now() - hold window
--   ORDER BY created_at DESC LIMIT 1
-- Runs on every CreatePurchase and on every receipt photo a customer sends.
-- The existing idx_purchase_customer_status_paid (customer_id, status, paid_at DESC) and
-- idx_purchase_invoice_status (invoice_type, status) cannot serve this predicate set, so
-- Postgres read the customer's rows and filtered them.
CREATE INDEX IF NOT EXISTS idx_purchase_customer_awaiting
    ON purchase (customer_id, created_at DESC)
    WHERE status IN ('pending', 'new');
