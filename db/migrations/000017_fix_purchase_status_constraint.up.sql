-- Migration 000017: Fix purchase status constraint
-- Identified by user reporting check constraint violation during purchase creation.
-- Go code uses 'new' and 'cancel' which were missing from the previous constraint.

BEGIN;

ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_status_valid;

ALTER TABLE purchase
    ADD CONSTRAINT purchase_status_valid
    CHECK (status IN ('new', 'pending', 'paid', 'failed', 'refunded', 'cancel'));

COMMIT;
