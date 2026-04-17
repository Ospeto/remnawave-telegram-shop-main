ALTER TABLE subscription_key
    ADD COLUMN IF NOT EXISTS auto_renew_plan_days INT,
    ADD COLUMN IF NOT EXISTS auto_renew_claimed_at TIMESTAMPTZ;

-- Backfill from the latest successful extension purchase for each key.
WITH latest_extension AS (
    SELECT DISTINCT ON (p.extend_key_id)
        p.extend_key_id AS key_id,
        p.days,
        p.traffic_limit_gb
    FROM purchase p
    WHERE p.status = 'paid'
      AND p.extend_key_id IS NOT NULL
      AND p.days > 0
    ORDER BY p.extend_key_id, COALESCE(p.paid_at, p.created_at) DESC, p.id DESC
)
UPDATE subscription_key k
SET auto_renew_plan_days = latest_extension.days,
    traffic_limit_gb = latest_extension.traffic_limit_gb
FROM latest_extension
WHERE k.id = latest_extension.key_id;

-- Backfill the remaining keys from the closest original paid purchase for the customer.
UPDATE subscription_key k
SET auto_renew_plan_days = src.days,
    traffic_limit_gb = src.traffic_limit_gb
FROM LATERAL (
    SELECT p.days, p.traffic_limit_gb
    FROM purchase p
    WHERE p.status = 'paid'
      AND p.customer_id = k.customer_id
      AND p.extend_key_id IS NULL
      AND p.days > 0
      AND (
          k.traffic_limit_gb = 0
          OR p.traffic_limit_gb = k.traffic_limit_gb
      )
    ORDER BY
        CASE WHEN p.traffic_limit_gb = k.traffic_limit_gb THEN 0 ELSE 1 END,
        ABS(EXTRACT(EPOCH FROM (COALESCE(p.paid_at, p.created_at) - k.created_at))) ASC,
        COALESCE(p.paid_at, p.created_at) DESC,
        p.id DESC
    LIMIT 1
) AS src
WHERE k.auto_renew_plan_days IS NULL;
