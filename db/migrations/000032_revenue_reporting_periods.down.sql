DROP VIEW IF EXISTS revenue_weekly;
DROP VIEW IF EXISTS revenue_daily;
DROP VIEW IF EXISTS revenue_monthly;

CREATE VIEW revenue_daily AS
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

CREATE VIEW revenue_monthly AS
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
