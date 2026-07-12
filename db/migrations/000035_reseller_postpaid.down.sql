-- 000035_reseller_postpaid.down.sql
DROP TABLE IF EXISTS reseller_ledger_entry;
DROP TABLE IF EXISTS reseller_credit_account;

ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_invoice_type_valid;
ALTER TABLE purchase ADD CONSTRAINT purchase_invoice_type_valid
  CHECK (invoice_type IN ('crypto','mobile_banking','wallet_topup','wallet_payment','yookasa'));
