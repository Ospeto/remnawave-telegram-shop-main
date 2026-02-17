-- Disable triggers to avoid foreign key checks during truncation if necessary,
-- but standard TRUNCATE ... CASCADE is safer and easier.

BEGIN;

-- Clear transaction/verification logs first
TRUNCATE TABLE mobile_payment_verification CASCADE;

-- Clear subscription keys
TRUNCATE TABLE subscription_key CASCADE;

-- Clear purchases (and referral bonuses linked to them if any)
TRUNCATE TABLE purchase CASCADE;

-- Clear promo codes
TRUNCATE TABLE promo_codes CASCADE;

-- Clear referrals
TRUNCATE TABLE referral CASCADE;

-- Clear customers last (this effectively resets all user accounts from the bot's perspective)
TRUNCATE TABLE customer CASCADE;

COMMIT;
