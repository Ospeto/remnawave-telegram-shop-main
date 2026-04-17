BEGIN;

ALTER TABLE wallet_transaction
    DROP CONSTRAINT IF EXISTS wallet_tx_referral_bonus_kind_valid,
    DROP COLUMN IF EXISTS referral_bonus_kind;

COMMIT;
