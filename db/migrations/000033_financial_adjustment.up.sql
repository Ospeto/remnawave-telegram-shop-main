CREATE TABLE IF NOT EXISTS financial_adjustment (
    id              BIGSERIAL PRIMARY KEY,
    purchase_id     BIGINT NULL REFERENCES purchase(id) ON DELETE SET NULL,
    adjustment_type TEXT NOT NULL,
    amount          NUMERIC(18, 2) NOT NULL CHECK (amount > 0),
    currency        TEXT NOT NULL DEFAULT 'MMK',
    effective_at    TIMESTAMPTZ NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    external_ref    TEXT NOT NULL DEFAULT '',
    created_by      TEXT NOT NULL DEFAULT 'system',
    idempotency_key TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT financial_adjustment_type_check
        CHECK (adjustment_type IN ('refund'))
);

CREATE UNIQUE INDEX IF NOT EXISTS financial_adjustment_idempotency_key_uidx
    ON financial_adjustment (idempotency_key);

CREATE INDEX IF NOT EXISTS financial_adjustment_effective_at_idx
    ON financial_adjustment (effective_at);

CREATE INDEX IF NOT EXISTS financial_adjustment_purchase_id_idx
    ON financial_adjustment (purchase_id);

CREATE INDEX IF NOT EXISTS financial_adjustment_type_effective_idx
    ON financial_adjustment (adjustment_type, effective_at);
