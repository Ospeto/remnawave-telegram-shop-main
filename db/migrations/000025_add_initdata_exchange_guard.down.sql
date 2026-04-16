BEGIN;

DROP INDEX IF EXISTS idx_telegram_init_data_exchange_expires_at;
DROP TABLE IF EXISTS telegram_init_data_exchange;

COMMIT;
