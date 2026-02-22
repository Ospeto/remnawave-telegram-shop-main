-- Add wallet support to customer table
ALTER TABLE customer 
    ADD COLUMN IF NOT EXISTS balance DECIMAL(18,2) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS auto_renew BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS auto_renew_duration INT NOT NULL DEFAULT 30;

-- Create wallet transactions table for history
CREATE TABLE IF NOT EXISTS wallet_transaction (
    id BIGSERIAL PRIMARY KEY,
    customer_id BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    amount DECIMAL(18,2) NOT NULL,
    type VARCHAR(20) NOT NULL, -- 'topup', 'purchase', 'refund'
    purchase_id BIGINT REFERENCES purchase(id) ON DELETE SET NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_wallet_transaction_customer ON wallet_transaction(customer_id);
CREATE INDEX IF NOT EXISTS idx_wallet_transaction_created_at ON wallet_transaction(created_at);

-- Add new invoice types for wallet operations
-- Note: These are handled in code, but documenting here:
-- 'wallet_topup' - For adding funds to wallet
-- 'wallet_payment' - For purchasing with wallet balance
