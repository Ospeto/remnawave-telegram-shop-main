-- 000035_reseller_postpaid.up.sql

ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_invoice_type_valid;
ALTER TABLE purchase ADD CONSTRAINT purchase_invoice_type_valid
  CHECK (invoice_type IN ('crypto','mobile_banking','wallet_topup','wallet_payment','yookasa','postpaid'));

CREATE TABLE IF NOT EXISTS reseller_credit_account (
  customer_id   BIGINT PRIMARY KEY REFERENCES customer(id),
  credit_limit  NUMERIC(18,2) NOT NULL DEFAULT 0,
  balance_owed  NUMERIC(18,2) NOT NULL DEFAULT 0,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT reseller_credit_account_limit_nonneg CHECK (credit_limit >= 0),
  CONSTRAINT reseller_credit_account_owed_nonneg CHECK (balance_owed >= 0)
);

CREATE TABLE IF NOT EXISTS reseller_ledger_entry (
  id               BIGSERIAL PRIMARY KEY,
  customer_id      BIGINT NOT NULL REFERENCES customer(id),
  entry_type       TEXT NOT NULL,
  direction        TEXT NOT NULL,
  amount           NUMERIC(18,2) NOT NULL,
  purchase_id      BIGINT NULL REFERENCES purchase(id),
  effective_at     TIMESTAMPTZ NOT NULL,
  note             TEXT NULL,
  created_by       TEXT NOT NULL,
  idempotency_key  TEXT NOT NULL,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT reseller_ledger_entry_type_check
    CHECK (entry_type IN ('sale','settlement','adjustment')),
  CONSTRAINT reseller_ledger_entry_direction_check
    CHECK (direction IN ('increase','decrease')),
  CONSTRAINT reseller_ledger_entry_amount_positive CHECK (amount > 0),
  CONSTRAINT reseller_ledger_entry_sale_purchase_required
    CHECK (
      (entry_type = 'sale' AND purchase_id IS NOT NULL AND direction = 'increase')
      OR (entry_type = 'settlement' AND direction = 'decrease')
      OR (entry_type = 'adjustment')
    ),
  CONSTRAINT reseller_ledger_entry_idempotency_key_unique UNIQUE (idempotency_key)
);

CREATE INDEX IF NOT EXISTS reseller_ledger_entry_customer_effective_idx
  ON reseller_ledger_entry (customer_id, effective_at DESC, id DESC);
