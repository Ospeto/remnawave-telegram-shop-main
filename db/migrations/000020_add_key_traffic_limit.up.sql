-- Add traffic_limit_gb to subscription_key so auto-renew cron knows
-- whether a key is unlimited (0) or limited (> 0) without querying purchases.
ALTER TABLE subscription_key
    ADD COLUMN IF NOT EXISTS traffic_limit_gb INT NOT NULL DEFAULT 0;
