-- Re-assert the referral FK target on customer(id).

BEGIN;

ALTER TABLE referral DROP CONSTRAINT IF EXISTS referral_referrer_id_fkey;
ALTER TABLE referral DROP CONSTRAINT IF EXISTS referral_referee_id_fkey;

ALTER TABLE referral
    ADD CONSTRAINT referral_referrer_id_fkey
        FOREIGN KEY (referrer_id)
            REFERENCES customer(id)
            ON DELETE CASCADE;

ALTER TABLE referral
    ADD CONSTRAINT referral_referee_id_fkey
        FOREIGN KEY (referee_id)
            REFERENCES customer(id)
            ON DELETE CASCADE;

COMMIT;
