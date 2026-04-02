BEGIN;

ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_status_valid;

ALTER TABLE purchase
    ADD CONSTRAINT purchase_status_valid
    CHECK (status IN ('new', 'pending', 'processing', 'paid', 'failed', 'refunded', 'cancel'));

COMMIT;
