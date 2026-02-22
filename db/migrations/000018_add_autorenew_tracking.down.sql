ALTER TABLE customer DROP COLUMN IF EXISTS last_auto_renewed_at;
ALTER TABLE customer DROP COLUMN IF EXISTS auto_renew_notified_at;
ALTER TABLE customer DROP COLUMN IF EXISTS auto_renew_traffic_gb;
