-- Rollback for migration 000015

BEGIN;

-- Restore revenue views (without timezone awareness)
CREATE OR REPLACE VIEW revenue_daily AS
SELECT
    DATE(paid_at) AS day,
    payment_method,
    currency,
    COUNT(*) AS total_purchases,
    SUM(amount) AS total_revenue,
    COUNT(DISTINCT customer_id) AS unique_customers
FROM purchase
WHERE status = 'paid' AND paid_at IS NOT NULL
GROUP BY DATE(paid_at), payment_method, currency
ORDER BY day DESC;

CREATE OR REPLACE VIEW revenue_monthly AS
SELECT
    DATE_TRUNC('month', paid_at)::DATE AS month,
    payment_method,
    currency,
    COUNT(*) AS total_purchases,
    SUM(amount) AS total_revenue,
    COUNT(DISTINCT customer_id) AS unique_customers
FROM purchase
WHERE status = 'paid' AND paid_at IS NOT NULL
GROUP BY DATE_TRUNC('month', paid_at), payment_method, currency
ORDER BY month DESC;

-- Drop new indexes
DROP INDEX IF EXISTS idx_mpv_transaction_id;
DROP INDEX IF EXISTS idx_referral_bonus_pending;

-- Revert promo_codes timestamps
ALTER TABLE promo_codes
    ALTER COLUMN valid_until TYPE TIMESTAMP USING valid_until AT TIME ZONE 'UTC',
    ALTER COLUMN created_at  TYPE TIMESTAMP USING created_at  AT TIME ZONE 'UTC';

-- Revert promo_codes PK type (best-effort; may fail if rows exceed INT range)
ALTER TABLE purchase    ALTER COLUMN promo_code_id TYPE INTEGER;
ALTER TABLE promo_codes ALTER COLUMN id TYPE INTEGER;

-- Remove referral constraints and restore original columns
ALTER TABLE referral
    DROP CONSTRAINT IF EXISTS referral_referrer_id_fkey,
    DROP CONSTRAINT IF EXISTS referral_referee_id_fkey,
    DROP CONSTRAINT IF EXISTS referral_referee_unique;

ALTER TABLE referral
    ADD COLUMN referrer_id_old BIGINT REFERENCES customer(telegram_id),
    ADD COLUMN referee_id_old  BIGINT REFERENCES customer(telegram_id);

UPDATE referral r
SET referrer_id_old = c.telegram_id
FROM customer c WHERE c.id = r.referrer_id;

UPDATE referral r
SET referee_id_old = c.telegram_id
FROM customer c WHERE c.id = r.referee_id;

ALTER TABLE referral
    DROP COLUMN referrer_id,
    DROP COLUMN referee_id;

ALTER TABLE referral
    RENAME COLUMN referrer_id_old TO referrer_id;
ALTER TABLE referral
    RENAME COLUMN referee_id_old TO referee_id;

-- Remove CHECK constraints
ALTER TABLE wallet_transaction DROP CONSTRAINT IF EXISTS wallet_tx_type_valid;
ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_invoice_type_valid;
ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_status_valid;

-- Revert balance type
ALTER TABLE wallet_transaction ALTER COLUMN amount TYPE DECIMAL(20,8);
ALTER TABLE customer ALTER COLUMN balance DROP NOT NULL;
ALTER TABLE customer ALTER COLUMN balance TYPE DECIMAL(20,8);

COMMIT;
