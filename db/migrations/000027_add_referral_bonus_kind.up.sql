BEGIN;

ALTER TABLE wallet_transaction
    ADD COLUMN IF NOT EXISTS referral_bonus_kind TEXT,
    DROP CONSTRAINT IF EXISTS wallet_tx_referral_bonus_kind_valid,
    ADD CONSTRAINT wallet_tx_referral_bonus_kind_valid
        CHECK (referral_bonus_kind IS NULL OR referral_bonus_kind IN ('referrer', 'referee'));

UPDATE wallet_transaction
SET referral_bonus_kind = 'referrer'
WHERE type = 'referral'
  AND referral_bonus_kind IS NULL
  AND description = 'Referral bonus — friend made their first purchase';

UPDATE wallet_transaction
SET referral_bonus_kind = 'referee'
WHERE type = 'referral'
  AND referral_bonus_kind IS NULL
  AND description = 'Welcome bonus — joined via referral link';

COMMIT;
