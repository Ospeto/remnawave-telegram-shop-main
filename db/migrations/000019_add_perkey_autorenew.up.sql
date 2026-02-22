-- Per-key auto-renew: each subscription key can independently be set to auto-renew.
-- When auto_renew=true, the cron will EXTEND this specific key rather than creating a new one.
-- This correctly handles users with multiple keys.

ALTER TABLE subscription_key
  ADD COLUMN IF NOT EXISTS auto_renew             BOOLEAN       DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS last_auto_renewed_at   TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS auto_renew_notified_at TIMESTAMPTZ;

-- Index for the cron query: find auto_renew=true keys expiring soon
CREATE INDEX IF NOT EXISTS idx_sub_key_autorenew_expiry
  ON subscription_key (auto_renew, expire_at)
  WHERE auto_renew = TRUE;
