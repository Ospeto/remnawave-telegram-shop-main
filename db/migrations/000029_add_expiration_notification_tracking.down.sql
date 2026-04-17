DROP INDEX IF EXISTS idx_sub_key_expiration_notification;

ALTER TABLE subscription_key
    DROP COLUMN IF EXISTS expiration_notified_at;
