BEGIN;

CREATE TABLE IF NOT EXISTS telegram_init_data_exchange (
    binding_key TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_telegram_init_data_exchange_expires_at
    ON telegram_init_data_exchange (expires_at);

COMMIT;
