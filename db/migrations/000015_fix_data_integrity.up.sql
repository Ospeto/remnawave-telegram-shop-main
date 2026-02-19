-- Migration 000015: Fix all data integrity, constraint, and timezone issues
-- Identified by database skills evaluation (DB-3 through DB-12)

BEGIN;

-- ── DB-3: Normalize balance column that was added differently in 000012 vs 000013 ──────────────
-- On a fresh install 000013 wins (NOT NULL + DEFAULT 0, DECIMAL(18,2)).
-- On an existing install 000012 won (nullable, DECIMAL(20,8)).
-- This migration normalises both paths to the same definition.
ALTER TABLE customer
    ALTER COLUMN balance SET NOT NULL,
    ALTER COLUMN balance SET DEFAULT 0,
    ALTER COLUMN balance TYPE NUMERIC(18,2) USING COALESCE(balance, 0)::NUMERIC(18,2);

ALTER TABLE wallet_transaction
    ALTER COLUMN amount TYPE NUMERIC(18,2) USING amount::NUMERIC(18,2);

-- ── DB-5: Constrain purchase.status and invoice_type ─────────────────────────────────────────
-- Existing data should already match these values; if not, the constraint will fail and alert us.
  -- REMOVED: UPDATE statements caused 'pending trigger events' conflict with ALTER TABLE.
  -- The CHECK constraint addition below will fail safely if invalid data exists.

ALTER TABLE purchase
    ADD CONSTRAINT purchase_status_valid
        CHECK (status IN ('pending','paid','failed','refunded'));

ALTER TABLE purchase
    ADD CONSTRAINT purchase_invoice_type_valid
        CHECK (invoice_type IN ('crypto','mobile_banking','wallet_topup','wallet_payment','yookasa'));

-- ── DB-6: Constrain wallet_transaction.type ──────────────────────────────────────────────────
ALTER TABLE wallet_transaction
    ADD CONSTRAINT wallet_tx_type_valid
        CHECK (type IN ('topup','purchase','refund'));

-- ── DB-4: Prevent duplicate referral entries ─────────────────────────────────────────────────
-- A referee can only be referred once. Without this, one person can grant multiple bonuses.
-- Remove existing duplicates (keep the oldest), then add constraint.
DELETE FROM referral r1
USING referral r2
WHERE r1.referee_id = r2.referee_id
  AND r1.id > r2.id;

ALTER TABLE referral
    ADD CONSTRAINT referral_referee_unique UNIQUE (referee_id);

-- ── DB-2: Fix referral FKs to point at customer.id instead of customer.telegram_id ────────────
-- Currently: referrer_id + referee_id FK → customer(telegram_id)
-- They should reference customer(id) to be consistent with all other tables.
-- First drop old FK constraints (they reference telegram_id).
ALTER TABLE referral
    DROP CONSTRAINT IF EXISTS referral_referrer_id_fkey,
    DROP CONSTRAINT IF EXISTS referral_referee_id_fkey;

-- Add new columns referencing customer.id
ALTER TABLE referral
    ADD COLUMN referrer_customer_id BIGINT,
    ADD COLUMN referee_customer_id  BIGINT;

-- Backfill: resolve telegram_id → customer.id
UPDATE referral r
SET referrer_customer_id = c.id
FROM customer c
WHERE c.telegram_id = r.referrer_id;

UPDATE referral r
SET referee_customer_id = c.id
FROM customer c
WHERE c.telegram_id = r.referee_id;

-- Apply NOT NULL and FK constraints on the new columns
ALTER TABLE referral
    ALTER COLUMN referrer_customer_id SET NOT NULL,
    ALTER COLUMN referee_customer_id  SET NOT NULL;

ALTER TABLE referral
    ADD CONSTRAINT referral_referrer_id_fkey
        FOREIGN KEY (referrer_customer_id) REFERENCES customer(id) ON DELETE CASCADE,
    ADD CONSTRAINT referral_referee_id_fkey
        FOREIGN KEY (referee_customer_id) REFERENCES customer(id) ON DELETE CASCADE;

-- Drop old columns
ALTER TABLE referral
    DROP COLUMN referrer_id,
    DROP COLUMN referee_id;

-- Rename new columns to original names for backward-compat in Go structs
ALTER TABLE referral
    RENAME COLUMN referrer_customer_id TO referrer_id;
ALTER TABLE referral
    RENAME COLUMN referee_customer_id  TO referee_id;

-- ── DB-8: Normalise promo_codes primary key to BIGINT ────────────────────────────────────────
-- SERIAL uses INTEGER (max ~2.1B). All other tables use BIGSERIAL/BIGINT.
ALTER TABLE promo_codes ALTER COLUMN id TYPE BIGINT;
ALTER TABLE purchase   ALTER COLUMN promo_code_id TYPE BIGINT;

-- ── DB-9: Fix promo_codes timestamp columns to use TIMESTAMPTZ ───────────────────────────────
ALTER TABLE promo_codes
    ALTER COLUMN valid_until TYPE TIMESTAMPTZ USING valid_until AT TIME ZONE 'UTC',
    ALTER COLUMN created_at  TYPE TIMESTAMPTZ USING created_at  AT TIME ZONE 'UTC';

-- ── DB-10: Index for mobile payment replay-attack check ──────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_mpv_transaction_id
    ON mobile_payment_verification(transaction_id);

-- ── DB-11: Partial index for unprocessed referral bonuses ────────────────────────────────────
CREATE INDEX IF NOT EXISTS idx_referral_bonus_pending
    ON referral(bonus_granted) WHERE bonus_granted = false;

-- ── DB-12: Fix revenue views to use explicit timezone ────────────────────────────────────────
-- Using 'Asia/Yangon' (+06:30) which matches the shop's operating timezone.
-- Change this if your server operates in a different timezone.
CREATE OR REPLACE VIEW revenue_daily AS
SELECT
    (paid_at AT TIME ZONE 'Asia/Yangon')::DATE AS day,
    payment_method,
    currency,
    COUNT(*)                        AS total_purchases,
    SUM(amount)                     AS total_revenue,
    COUNT(DISTINCT customer_id)     AS unique_customers
FROM purchase
WHERE status = 'paid' AND paid_at IS NOT NULL
GROUP BY 1, 2, 3
ORDER BY 1 DESC;

CREATE OR REPLACE VIEW revenue_monthly AS
SELECT
    DATE_TRUNC('month', paid_at AT TIME ZONE 'Asia/Yangon')::DATE AS month,
    payment_method,
    currency,
    COUNT(*)                        AS total_purchases,
    SUM(amount)                     AS total_revenue,
    COUNT(DISTINCT customer_id)     AS unique_customers
FROM purchase
WHERE status = 'paid' AND paid_at IS NOT NULL
GROUP BY 1, 2, 3
ORDER BY 1 DESC;

COMMIT;
