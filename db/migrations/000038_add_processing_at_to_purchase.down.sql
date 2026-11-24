-- 000038_add_processing_at_to_purchase.down.sql

DROP INDEX IF EXISTS idx_purchase_stale_processing;

ALTER TABLE purchase
  DROP COLUMN IF EXISTS processing_at;
