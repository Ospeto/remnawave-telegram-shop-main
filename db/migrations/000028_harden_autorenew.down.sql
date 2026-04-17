ALTER TABLE subscription_key
    DROP COLUMN IF EXISTS auto_renew_claimed_at,
    DROP COLUMN IF EXISTS auto_renew_plan_days;
