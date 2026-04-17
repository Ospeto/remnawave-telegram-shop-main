BEGIN;

ALTER TABLE referral
    ADD COLUMN IF NOT EXISTS bonus_granted_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS referee_bonus_granted_at TIMESTAMPTZ;

UPDATE referral
SET bonus_granted_at = used_at
WHERE bonus_granted = TRUE
  AND bonus_granted_at IS NULL;

UPDATE referral
SET referee_bonus_granted_at = used_at
WHERE referee_bonus_granted = TRUE
  AND referee_bonus_granted_at IS NULL;

COMMIT;
