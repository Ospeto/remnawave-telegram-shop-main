-- Fix: referral FK constraints point to customer(id) but should point to customer(telegram_id)
-- The referral table stores telegram_id values, not customer primary key values.

BEGIN;

ALTER TABLE referral DROP CONSTRAINT IF EXISTS referral_referrer_id_fkey;
ALTER TABLE referral DROP CONSTRAINT IF EXISTS referral_referee_id_fkey;

ALTER TABLE referral
    ADD CONSTRAINT referral_referrer_id_fkey
        FOREIGN KEY (referrer_id)
            REFERENCES customer(telegram_id)
            ON DELETE CASCADE;

ALTER TABLE referral
    ADD CONSTRAINT referral_referee_id_fkey
        FOREIGN KEY (referee_id)
            REFERENCES customer(telegram_id)
            ON DELETE CASCADE;

COMMIT;
