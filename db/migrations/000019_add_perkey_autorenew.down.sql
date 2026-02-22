DROP INDEX IF EXISTS idx_sub_key_autorenew_expiry;
ALTER TABLE subscription_key
  DROP COLUMN IF EXISTS auto_renew,
  DROP COLUMN IF EXISTS last_auto_renewed_at,
  DROP COLUMN IF EXISTS auto_renew_notified_at;
