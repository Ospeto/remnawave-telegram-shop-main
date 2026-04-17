BEGIN;

ALTER TABLE referral
    DROP COLUMN IF EXISTS bonus_granted_at,
    DROP COLUMN IF EXISTS referee_bonus_granted_at;

COMMIT;
