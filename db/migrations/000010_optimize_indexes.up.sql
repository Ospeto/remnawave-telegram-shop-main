-- 1. Fix Critical Performance: Index for customer purchases lookup
-- This speeds up FindSuccessfulPaidPurchaseByCustomer significantly
CREATE INDEX IF NOT EXISTS idx_purchase_customer_id ON purchase(customer_id);

-- 2. Prevent Double Payments: Unique constraint on transactions
-- Only enforce uniqueness for non-empty transaction IDs
CREATE UNIQUE INDEX IF NOT EXISTS idx_purchase_txn_id ON purchase(transaction_id) 
WHERE transaction_id != '';

-- 3. Enforce User Uniqueness: Explicit unique index
-- Drop old hash index if it exists (from 000001) as we want a B-tree unique index
DROP INDEX IF EXISTS idx_customer_telegram_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_telegram_id_unique ON customer(telegram_id);
