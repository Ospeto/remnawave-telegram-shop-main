-- db/migrations/000034_reseller_wholesale.up.sql
ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS is_reseller BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS pricing_tier TEXT NOT NULL DEFAULT 'retail';

ALTER TABLE purchase
    DROP CONSTRAINT IF EXISTS purchase_pricing_tier_check;

ALTER TABLE purchase
    ADD CONSTRAINT purchase_pricing_tier_check
    CHECK (pricing_tier IN ('retail', 'wholesale'));
