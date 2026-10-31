-- 000036_add_hidden_to_subscription_key.up.sql

ALTER TABLE subscription_key
  ADD COLUMN IF NOT EXISTS is_hidden BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_subscription_key_customer_hidden
  ON subscription_key (customer_id, is_hidden);
