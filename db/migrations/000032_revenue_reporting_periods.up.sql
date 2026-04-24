-- Split service revenue from cash movement and add a weekly reporting view.
-- Wallet top-ups are cash collected, but not service revenue until the wallet is spent.

DROP VIEW IF EXISTS revenue_daily;
DROP VIEW IF EXISTS revenue_monthly;
DROP VIEW IF EXISTS revenue_weekly;

CREATE VIEW revenue_daily AS
WITH paid AS (
    SELECT
        (p.paid_at AT TIME ZONE 'Asia/Yangon')::DATE AS day,
        COALESCE(
            NULLIF(p.payment_method, ''),
            CASE WHEN p.invoice_type = 'wallet_payment' THEN 'wallet' ELSE 'unknown' END
        ) AS payment_method,
        COALESCE(NULLIF(p.currency, ''), 'MMK') AS currency,
        p.amount,
        p.customer_id,
        p.invoice_type,
        p.extend_key_id
    FROM purchase p
    WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
)
SELECT
    day,
    payment_method,
    currency,
    COUNT(*) AS total_purchases,
    COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS total_revenue,
    COUNT(DISTINCT customer_id) AS unique_customers,
    COALESCE(SUM(CASE WHEN invoice_type IN ('crypto', 'mobile_banking', 'wallet_topup') THEN amount ELSE 0 END), 0) AS cash_collected,
    COALESCE(SUM(CASE WHEN invoice_type = 'wallet_topup' THEN amount ELSE 0 END), 0) AS wallet_topups,
    COALESCE(SUM(CASE WHEN invoice_type = 'wallet_payment' THEN amount ELSE 0 END), 0) AS wallet_spend,
    COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS service_revenue,
    COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NULL) AS new_key_purchases,
    COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NOT NULL) AS extension_purchases,
    COUNT(*) FILTER (WHERE invoice_type = 'wallet_topup') AS wallet_topup_purchases
FROM paid
GROUP BY 1, 2, 3
ORDER BY 1 DESC;

CREATE VIEW revenue_monthly AS
WITH paid AS (
    SELECT
        DATE_TRUNC('month', p.paid_at AT TIME ZONE 'Asia/Yangon')::DATE AS month,
        COALESCE(
            NULLIF(p.payment_method, ''),
            CASE WHEN p.invoice_type = 'wallet_payment' THEN 'wallet' ELSE 'unknown' END
        ) AS payment_method,
        COALESCE(NULLIF(p.currency, ''), 'MMK') AS currency,
        p.amount,
        p.customer_id,
        p.invoice_type,
        p.extend_key_id
    FROM purchase p
    WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
)
SELECT
    month,
    payment_method,
    currency,
    COUNT(*) AS total_purchases,
    COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS total_revenue,
    COUNT(DISTINCT customer_id) AS unique_customers,
    COALESCE(SUM(CASE WHEN invoice_type IN ('crypto', 'mobile_banking', 'wallet_topup') THEN amount ELSE 0 END), 0) AS cash_collected,
    COALESCE(SUM(CASE WHEN invoice_type = 'wallet_topup' THEN amount ELSE 0 END), 0) AS wallet_topups,
    COALESCE(SUM(CASE WHEN invoice_type = 'wallet_payment' THEN amount ELSE 0 END), 0) AS wallet_spend,
    COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS service_revenue,
    COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NULL) AS new_key_purchases,
    COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NOT NULL) AS extension_purchases,
    COUNT(*) FILTER (WHERE invoice_type = 'wallet_topup') AS wallet_topup_purchases
FROM paid
GROUP BY 1, 2, 3
ORDER BY 1 DESC;

CREATE VIEW revenue_weekly AS
WITH paid AS (
    SELECT
        DATE_TRUNC('week', p.paid_at AT TIME ZONE 'Asia/Yangon')::DATE AS week,
        COALESCE(
            NULLIF(p.payment_method, ''),
            CASE WHEN p.invoice_type = 'wallet_payment' THEN 'wallet' ELSE 'unknown' END
        ) AS payment_method,
        COALESCE(NULLIF(p.currency, ''), 'MMK') AS currency,
        p.amount,
        p.customer_id,
        p.invoice_type,
        p.extend_key_id
    FROM purchase p
    WHERE p.status = 'paid' AND p.paid_at IS NOT NULL
)
SELECT
    week,
    payment_method,
    currency,
    COUNT(*) AS total_purchases,
    COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS total_revenue,
    COUNT(DISTINCT customer_id) AS unique_customers,
    COALESCE(SUM(CASE WHEN invoice_type IN ('crypto', 'mobile_banking', 'wallet_topup') THEN amount ELSE 0 END), 0) AS cash_collected,
    COALESCE(SUM(CASE WHEN invoice_type = 'wallet_topup' THEN amount ELSE 0 END), 0) AS wallet_topups,
    COALESCE(SUM(CASE WHEN invoice_type = 'wallet_payment' THEN amount ELSE 0 END), 0) AS wallet_spend,
    COALESCE(SUM(CASE WHEN invoice_type <> 'wallet_topup' THEN amount ELSE 0 END), 0) AS service_revenue,
    COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NULL) AS new_key_purchases,
    COUNT(*) FILTER (WHERE invoice_type <> 'wallet_topup' AND extend_key_id IS NOT NULL) AS extension_purchases,
    COUNT(*) FILTER (WHERE invoice_type = 'wallet_topup') AS wallet_topup_purchases
FROM paid
GROUP BY 1, 2, 3
ORDER BY 1 DESC;
