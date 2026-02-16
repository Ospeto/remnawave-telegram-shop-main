CREATE TABLE IF NOT EXISTS promo_codes (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) NOT NULL UNIQUE,
    discount_percent INTEGER NOT NULL CHECK (discount_percent > 0 AND discount_percent <= 100),
    max_uses INTEGER NOT NULL CHECK (max_uses > 0),
    used_count INTEGER NOT NULL DEFAULT 0,
    valid_until TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

ALTER TABLE purchase ADD COLUMN IF NOT EXISTS promo_code_id INTEGER REFERENCES promo_codes(id);
