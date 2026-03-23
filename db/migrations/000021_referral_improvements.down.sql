-- Revert wallet_transaction type check
ALTER TABLE wallet_transaction
    DROP CONSTRAINT IF EXISTS wallet_tx_type_valid;

ALTER TABLE wallet_transaction
    ADD CONSTRAINT wallet_tx_type_valid
        CHECK (type IN ('topup', 'purchase', 'refund'));

-- Revert referral table changes
ALTER TABLE referral DROP COLUMN IF EXISTS referee_bonus_granted;
ALTER TABLE referral DROP CONSTRAINT IF EXISTS referral_referee_id_unique;
