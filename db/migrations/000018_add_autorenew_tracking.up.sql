-- Track when a user was last successfully auto-renewed (idempotency guard).
-- Prevents double-charging if the cron job fires twice in one day.
ALTER TABLE customer ADD COLUMN IF NOT EXISTS last_auto_renewed_at TIMESTAMPTZ;

-- Track when a "low balance" warning was last sent.
-- Prevents spamming the user if the cron runs daily but the balance is never topped up.
ALTER TABLE customer ADD COLUMN IF NOT EXISTS auto_renew_notified_at TIMESTAMPTZ;

-- Track the traffic type the customer wants to renew (unlimited=0, or GB limit).
-- Decouples plan selection from fragile "match by days only" lookup.
ALTER TABLE customer ADD COLUMN IF NOT EXISTS auto_renew_traffic_gb INTEGER DEFAULT 0;
