-- 000038_add_processing_at_to_purchase.up.sql
-- A9: a purchase stuck in 'processing' was invisible forever because nothing
-- recorded when the claim happened. processing_at lets the reaper decide which
-- in-flight rows are stale.

ALTER TABLE purchase
  ADD COLUMN IF NOT EXISTS processing_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_purchase_stale_processing
  ON purchase (processing_at)
  WHERE status = 'processing';
