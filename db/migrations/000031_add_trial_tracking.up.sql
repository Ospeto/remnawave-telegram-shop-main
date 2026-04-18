ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS trial_used_at TIMESTAMPTZ;

UPDATE customer c
SET trial_used_at = COALESCE(c.expire_at, c.created_at, NOW())
WHERE c.trial_used_at IS NULL
  AND (
    (c.subscription_link IS NOT NULL AND c.subscription_link <> '')
    OR EXISTS (
      SELECT 1
      FROM subscription_key sk
      WHERE sk.customer_id = c.id
    )
  );
