-- 000038_wallet_refund_unique.up.sql
-- A10: a wallet refund must be refused by the database, not by luck of timing.
-- One refund row per purchase is the invariant; the partial unique index makes a
-- concurrent second refundCredit fail with 23505 instead of double-crediting.

CREATE UNIQUE INDEX IF NOT EXISTS idx_wallet_transaction_refund_once
  ON wallet_transaction (purchase_id)
  WHERE type = 'refund' AND purchase_id IS NOT NULL;
