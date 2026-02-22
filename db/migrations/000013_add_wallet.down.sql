-- Down migration for wallet support
DROP TABLE IF EXISTS wallet_transaction;

ALTER TABLE customer 
    DROP COLUMN IF EXISTS balance,
    DROP COLUMN IF EXISTS auto_renew,
    DROP COLUMN IF EXISTS auto_renew_duration;
