DROP VIEW IF EXISTS revenue_monthly;
DROP VIEW IF EXISTS revenue_daily;

DROP INDEX IF EXISTS idx_purchase_paid_at;
DROP INDEX IF EXISTS idx_purchase_status;

ALTER TABLE purchase DROP COLUMN IF EXISTS plan_label;
ALTER TABLE purchase DROP COLUMN IF EXISTS payment_method;
ALTER TABLE purchase DROP COLUMN IF EXISTS payment_phone;
ALTER TABLE purchase DROP COLUMN IF EXISTS verified_at;
ALTER TABLE purchase DROP COLUMN IF EXISTS transaction_id;
