-- 000036_add_hidden_to_subscription_key.down.sql

DROP INDEX IF EXISTS idx_subscription_key_customer_hidden;

ALTER TABLE subscription_key
  DROP COLUMN IF EXISTS hidden_at,
  DROP COLUMN IF EXISTS is_hidden;
