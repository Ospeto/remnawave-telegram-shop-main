CREATE TABLE IF NOT EXISTS app_config (
    key VARCHAR(255) PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO app_config (key, value) VALUES ('referral_bonus_amount', '1000') ON CONFLICT DO NOTHING;
