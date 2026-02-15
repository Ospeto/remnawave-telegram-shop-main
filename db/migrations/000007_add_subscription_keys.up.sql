CREATE TABLE subscription_key (
    id               BIGSERIAL PRIMARY KEY,
    customer_id      BIGINT NOT NULL REFERENCES customer(id) ON DELETE CASCADE,
    remnawave_uuid   UUID NOT NULL,
    username         TEXT NOT NULL,
    subscription_url TEXT NOT NULL,
    expire_at        TIMESTAMP WITH TIME ZONE,
    status           VARCHAR(20) DEFAULT 'active',
    created_at       TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    label            TEXT DEFAULT ''
);

CREATE INDEX idx_sub_key_customer ON subscription_key(customer_id);
CREATE UNIQUE INDEX idx_sub_key_uuid ON subscription_key(remnawave_uuid);

-- Migrate existing customer subscription data into the new table
INSERT INTO subscription_key (customer_id, remnawave_uuid, username, subscription_url, expire_at, status, label)
SELECT
    c.id,
    gen_random_uuid(),
    COALESCE(c.telegram_id::text, 'unknown'),
    c.subscription_link,
    c.expire_at,
    CASE WHEN c.expire_at > NOW() THEN 'active' ELSE 'expired' END,
    'Key 1'
FROM customer c
WHERE c.subscription_link IS NOT NULL AND c.subscription_link != '';

-- Add extend_key_id to purchase table
ALTER TABLE purchase ADD COLUMN IF NOT EXISTS extend_key_id BIGINT REFERENCES subscription_key(id);
