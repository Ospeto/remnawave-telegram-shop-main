CREATE TABLE mobile_payment_verification (
    id               BIGSERIAL PRIMARY KEY,
    purchase_id      BIGINT REFERENCES purchase(id),
    transaction_id   TEXT NOT NULL UNIQUE,
    provider         VARCHAR(20) NOT NULL,
    phone_number     TEXT NOT NULL,
    amount           DECIMAL(20, 8),
    note             TEXT DEFAULT '',
    verified         BOOLEAN DEFAULT FALSE,
    rejection_reason TEXT DEFAULT '',
    created_at       TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mobile_payment_txn_id ON mobile_payment_verification(transaction_id);
CREATE INDEX idx_mobile_payment_purchase_id ON mobile_payment_verification(purchase_id);
