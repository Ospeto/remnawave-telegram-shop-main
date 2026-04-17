ALTER TABLE subscription_key
    ADD COLUMN IF NOT EXISTS expiration_notified_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_sub_key_expiration_notification
    ON subscription_key (status, expire_at, expiration_notified_at);
