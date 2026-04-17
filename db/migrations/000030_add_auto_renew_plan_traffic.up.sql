ALTER TABLE subscription_key
    ADD COLUMN IF NOT EXISTS auto_renew_plan_traffic_gb INTEGER;

WITH latest_extension AS (
    SELECT DISTINCT ON (p.extend_key_id)
        p.extend_key_id AS key_id,
        p.traffic_limit_gb
    FROM purchase p
    WHERE p.status = 'paid'
      AND p.extend_key_id IS NOT NULL
      AND p.days > 0
    ORDER BY p.extend_key_id, COALESCE(p.paid_at, p.created_at) DESC, p.id DESC
)
UPDATE subscription_key
SET auto_renew_plan_traffic_gb = latest_extension.traffic_limit_gb
FROM latest_extension
WHERE subscription_key.id = latest_extension.key_id
  AND subscription_key.auto_renew_plan_traffic_gb IS NULL;

WITH original_match AS (
    SELECT
        k.id AS key_id,
        src.traffic_limit_gb
    FROM subscription_key k
    JOIN LATERAL (
        SELECT p.traffic_limit_gb
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
    ) AS src ON TRUE
    WHERE k.auto_renew_plan_traffic_gb IS NULL
)
UPDATE subscription_key
SET auto_renew_plan_traffic_gb = original_match.traffic_limit_gb
FROM original_match
WHERE subscription_key.id = original_match.key_id
  AND subscription_key.auto_renew_plan_traffic_gb IS NULL;
