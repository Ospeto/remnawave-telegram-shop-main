-- db/migrations/000034_reseller_wholesale.down.sql
ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_pricing_tier_check;
ALTER TABLE purchase DROP COLUMN IF EXISTS pricing_tier;
ALTER TABLE customer DROP COLUMN IF EXISTS is_reseller;
