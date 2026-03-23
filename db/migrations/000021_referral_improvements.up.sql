-- Add unique constraint on referee_id to prevent duplicate referral rows
-- (race condition if user double-taps /start with a ref link)
ALTER TABLE referral ADD CONSTRAINT referral_referee_id_unique UNIQUE (referee_id);

-- Track that the new user (referee) also received their welcome bonus
ALTER TABLE referral ADD COLUMN referee_bonus_granted BOOLEAN NOT NULL DEFAULT FALSE;

-- Extend wallet_transaction type to include referral bonuses
ALTER TABLE wallet_transaction
    DROP CONSTRAINT IF EXISTS wallet_tx_type_valid;

ALTER TABLE wallet_transaction
    ADD CONSTRAINT wallet_tx_type_valid
        CHECK (type IN ('topup', 'purchase', 'refund', 'referral'));
