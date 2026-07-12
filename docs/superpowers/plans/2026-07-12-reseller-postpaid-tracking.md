# Reseller Postpaid Credit + Sales Ledger Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let approved resellers buy service plans on account (postpaid AR), fulfill immediately within credit limit, track sales/settlements in a dedicated ledger, and settle via wallet self-pay or admin offline recording.

**Architecture:** Separate AR tables (`reseller_credit_account`, `reseller_ledger_entry`) keep prepaid wallet non-negative. Postpaid purchases use new invoice type `postpaid`, resolve price via existing `ResolvePlanPrice`, create purchase + sale ledger + `balance_owed` under account row lock, then call existing `ProcessPurchaseById` for key fulfillment (no wallet debit, no receipt). Settlements decrease owed only; finance counts postpaid as service revenue on `paid_at` and settlement cash from ledger on settlement date.

**Tech Stack:** Go 1.25, Postgres 17, existing `internal/payment` + `internal/api` + React Mini App (`web-app/`).

## Global Constraints

- Do **not** turn wallet into credit or allow negative wallet balance.
- Do **not** re-enable Crypto Pay.
- Do **not** gift/assign keys or build downline tracking in v1.
- Money paths stay idempotent; `balance_owed` updates are transactional with ledger inserts.
- Do not casually refactor payment fulfillment or wallet mutation beyond postpaid amount/tier selection and AR side effects.
- Server-authoritative price and settlement amounts; no client-trusted balances.
- Promo remains blocked for resellers on all purchase paths.
- No historical AR backfill of prepaid wholesale purchases.
- Bot postpaid is **optional follow-up**, not required for launch.
- Self-pay rails v1: **wallet debit of settlement amount** + **admin offline**. Mobile-banking settlement invoice is deferred (optional follow-up).
- Adjustment rows use explicit `direction` column: `increase` | `decrease` (amount always > 0).
- Postpaid does **not** trigger referral conversion (keep `triggersReferralConversion` mobile_banking|wallet_payment only).

## Locked open choices (from design §Open)

| Topic | Choice |
|-------|--------|
| Self-pay rails | Wallet debit + admin offline (mobile banking settlement deferred) |
| Adjustment shape | `direction` TEXT CHECK (`increase`,`decrease`); amount > 0 |
| Bot postpaid | Out of v1 plan (optional follow-up task only if time) |

## File map

| File | Responsibility |
|------|----------------|
| `db/migrations/000035_reseller_postpaid.{up,down}.sql` | AR tables + invoice_type CHECK |
| `internal/database/reseller_credit.go` | Credit account + ledger repository |
| `internal/database/purchase.go` | `InvoiceTypePostpaid` constant |
| `internal/config/config.go` | `RESELLER_DEFAULT_CREDIT_LIMIT` |
| `internal/payment/payment.go` | `createPostpaidPurchase` branch |
| `internal/api/handlers.go` + `server.go` | parsePaymentMethod, purchase, reseller + admin AR APIs |
| `internal/reporting/*` or purchase revenue SQL | Settlement cash in finance |
| `web-app/src/pages/Checkout.tsx` | Postpaid payment option |
| `web-app/src/pages/ResellerAccount.tsx` | Own balance + ledger + pay |
| `web-app/src/pages/AdminResellers.tsx` | Limit / owed / settle |
| `web-app/src/App.tsx`, `Home.tsx` | Routes + Home card |
| `HOWTOUSE.md`, `docs/MINI_APP.md` | Ops docs |

---

### Task 1: Migration 000035 — AR tables + postpaid invoice type

**Files:**
- Create: `db/migrations/000035_reseller_postpaid.up.sql`
- Create: `db/migrations/000035_reseller_postpaid.down.sql`

**Interfaces:**
- Produces: tables `reseller_credit_account`, `reseller_ledger_entry`; purchase `invoice_type` allows `postpaid`

- [ ] **Step 1: Write up migration**

```sql
-- 000035_reseller_postpaid.up.sql

ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_invoice_type_check;
ALTER TABLE purchase ADD CONSTRAINT purchase_invoice_type_check
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
```

- [ ] **Step 2: Write down migration**

```sql
-- 000035_reseller_postpaid.down.sql
DROP TABLE IF EXISTS reseller_ledger_entry;
DROP TABLE IF EXISTS reseller_credit_account;

ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_invoice_type_check;
ALTER TABLE purchase ADD CONSTRAINT purchase_invoice_type_check
  CHECK (invoice_type IN ('crypto','mobile_banking','wallet_topup','wallet_payment','yookasa'));
```

- [ ] **Step 3: Commit**

```bash
git add db/migrations/000035_reseller_postpaid.up.sql db/migrations/000035_reseller_postpaid.down.sql
git commit -m "feat(db): add reseller AR credit account and postpaid invoice type (000035)"
```

---

### Task 2: InvoiceTypePostpaid constant

**Files:**
- Modify: `internal/database/purchase.go` (constants block ~17-24)
- Modify: `internal/database/db_fixes_test.go` (`TestInvoiceTypeConstants`)

**Interfaces:**
- Produces: `database.InvoiceTypePostpaid InvoiceType = "postpaid"`

- [ ] **Step 1: Write failing test**

In `db_fixes_test.go`, extend `TestInvoiceTypeConstants` to require:

```go
"postpaid": database.InvoiceTypePostpaid,
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/database/ -run TestInvoiceTypeConstants -count=1
```

Expected: FAIL — undefined `InvoiceTypePostpaid`

- [ ] **Step 3: Add constant**

```go
InvoiceTypePostpaid InvoiceType = "postpaid"
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/database/ -run TestInvoiceTypeConstants -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/purchase.go internal/database/db_fixes_test.go
git commit -m "feat(db): add InvoiceTypePostpaid constant"
```

---

### Task 3: Reseller credit account + ledger repository

**Files:**
- Create: `internal/database/reseller_credit.go`
- Create: `internal/database/reseller_credit_test.go`

**Interfaces:**
- Produces:
  - `type ResellerCreditAccount struct { CustomerID int64; CreditLimit float64; BalanceOwed float64; CreatedAt, UpdatedAt time.Time }`
  - `type ResellerLedgerEntryType string` — `sale|settlement|adjustment`
  - `type ResellerLedgerDirection string` — `increase|decrease`
  - `type ResellerLedgerEntry struct { ID int64; CustomerID int64; EntryType; Direction; Amount float64; PurchaseID *int64; EffectiveAt time.Time; Note, CreatedBy, IdempotencyKey string; CreatedAt time.Time }`
  - `type CreateLedgerEntryInput struct { CustomerID int64; EntryType ResellerLedgerEntryType; Direction ResellerLedgerDirection; Amount float64; PurchaseID *int64; EffectiveAt time.Time; Note, CreatedBy, IdempotencyKey string }`
  - `func NewResellerCreditRepository(pool *pgxpool.Pool) *ResellerCreditRepository`
  - `func (r *ResellerCreditRepository) EnsureAccount(ctx, customerID int64, defaultLimit float64) (*ResellerCreditAccount, error)` — insert if missing with limit=defaultLimit, owed=0
  - `func (r *ResellerCreditRepository) GetAccount(ctx, customerID int64) (*ResellerCreditAccount, error)` — nil if none
  - `func (r *ResellerCreditRepository) SetCreditLimit(ctx, customerID int64, limit float64) (*ResellerCreditAccount, error)` — ensure row, set limit, reject limit < 0 or limit < balance_owed
  - `func (r *ResellerCreditRepository) RecordSaleTx(ctx, tx pgx.Tx, in CreateLedgerEntryInput) (*ResellerLedgerEntry, *ResellerCreditAccount, bool created, error)` — FOR UPDATE account; require entry_type=sale, direction=increase, purchase_id set, amount>0; require balance_owed+amount <= credit_limit; insert ledger ON CONFLICT idempotency DO NOTHING; if inserted update balance_owed; if conflict reload by key and return created=false
  - `func (r *ResellerCreditRepository) RecordSettlementTx(ctx, tx pgx.Tx, in CreateLedgerEntryInput) (*ResellerLedgerEntry, *ResellerCreditAccount, bool created, error)` — FOR UPDATE; entry_type=settlement, direction=decrease; amount <= balance_owed; same idempotency pattern
  - `func (r *ResellerCreditRepository) RecordAdjustmentTx(ctx, tx pgx.Tx, in CreateLedgerEntryInput) (*ResellerLedgerEntry, *ResellerCreditAccount, bool created, error)` — admin only path; increase adds owed (cap not required for decrease); decrease requires amount <= owed
  - `func (r *ResellerCreditRepository) ListLedger(ctx, customerID int64, limit, offset int) ([]ResellerLedgerEntry, error)`
  - Stable errors: `ErrResellerInsufficientCredit`, `ErrResellerOverSettlement`, `ErrResellerCreditLimitBelowOwed`, `ErrResellerLedgerIdempotencyMismatch`

- [ ] **Step 1: Write failing unit tests** (table-driven, no live DB required for pure validation helpers; use fake tx interface if package already stubs pgx — otherwise structural + SQL-shape tests like financial_adjustment)

```go
func TestNormalizeResellerAmountRejectsNonPositive(t *testing.T) {
	if _, err := normalizeResellerAmount(0); err == nil {
		t.Fatal("expected error")
	}
	if _, err := normalizeResellerAmount(-1); err == nil {
		t.Fatal("expected error")
	}
}

func TestRemainingCredit(t *testing.T) {
	a := ResellerCreditAccount{CreditLimit: 10000, BalanceOwed: 2500}
	if got := a.RemainingCredit(); got != 7500 {
		t.Fatalf("RemainingCredit = %v, want 7500", got)
	}
}
```

- [ ] **Step 2: Run tests — expect FAIL**

```bash
go test ./internal/database/ -run 'TestNormalizeResellerAmount|TestRemainingCredit' -count=1
```

- [ ] **Step 3: Implement repository**

Key sale SQL pattern (inside tx):

```go
// 1) SELECT credit_limit, balance_owed FROM reseller_credit_account WHERE customer_id=$1 FOR UPDATE
// 2) if balance_owed + amount > credit_limit → ErrResellerInsufficientCredit
// 3) INSERT INTO reseller_ledger_entry (...) VALUES (...) ON CONFLICT (idempotency_key) DO NOTHING RETURNING ...
// 4) if no row returned: SELECT by idempotency_key; compare payload; return created=false
// 5) UPDATE reseller_credit_account SET balance_owed = balance_owed + $amount, updated_at=NOW() WHERE customer_id=$1
```

Settlement uses `balance_owed - amount` with `amount <= balance_owed`.

- [ ] **Step 4: Run package tests**

```bash
go test ./internal/database/ -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/reseller_credit.go internal/database/reseller_credit_test.go
git commit -m "feat(db): reseller credit account and AR ledger repository"
```

---

### Task 4: Default credit limit config

**Files:**
- Modify: `internal/config/config.go` (config struct + MustLoad env parse + getter)
- Modify: `.env.sample` (document key only if other money keys are listed there)
- Create or modify: `internal/config/config_test.go` (or existing config tests)

**Interfaces:**
- Produces: `func ResellerDefaultCreditLimit() float64` — from env `RESELLER_DEFAULT_CREDIT_LIMIT`, default `0`, reject negative at load (treat invalid as 0 or fail load — **fail load if set and < 0**)

- [ ] **Step 1: Failing test**

```go
func TestResellerDefaultCreditLimitDefaultZero(t *testing.T) {
	// after isolating env or reading getter with unset key
	if ResellerDefaultCreditLimit() < 0 {
		t.Fatal("limit must be non-negative")
	}
}
```

- [ ] **Step 2: Implement**

```go
// in config struct:
resellerDefaultCreditLimit float64

func ResellerDefaultCreditLimit() float64 {
	return conf.resellerDefaultCreditLimit
}

// in MustLoad / env parse:
// RESELLER_DEFAULT_CREDIT_LIMIT optional; empty → 0; parse float; if < 0 log fatal or clamp to 0 with slog.Error — prefer fatal on invalid
```

- [ ] **Step 3: Commit**

```bash
git add internal/config/config.go .env.sample
git commit -m "feat(config): RESELLER_DEFAULT_CREDIT_LIMIT"
```

---

### Task 5: Payment createPostpaidPurchase path

**Files:**
- Modify: `internal/payment/payment.go` (`createPurchaseWithOptionalExtend` switch ~1918, `resumeExistingPurchase` ~1215)
- Create: `internal/payment/postpaid_test.go`
- Wire: PaymentService needs `*database.ResellerCreditRepository` (add field + constructor param in `cmd/app` and any `NewPaymentService`)

**Interfaces:**
- Consumes: `ResellerCreditRepository.EnsureAccount`, `RecordSaleTx`, `InvoiceTypePostpaid`, `pricingTierFromContext`, `ProcessPurchaseById`
- Produces: postpaid branch in create switch; no wallet debit; no mobile pending block

**Design of createPostpaidPurchase:**

```go
func (s *PaymentService) createPostpaidPurchase(ctx context.Context, amount float64, days int, trafficLimitGB int, customer *database.Customer, promoID *int64, extendKeyID *int64) (string, int64, error) {
	if s.resellerCreditRepo == nil {
		return "", 0, fmt.Errorf("reseller credit repository is not configured")
	}
	if !customer.IsReseller {
		return "", 0, fmt.Errorf("postpaid is only available for resellers")
	}
	if amount <= 0 {
		return "", 0, fmt.Errorf("postpaid amount must be positive")
	}
	// Idempotent resume by key if present (same body match as other paths)
	// BeginTx:
	//   EnsureAccount with config.ResellerDefaultCreditLimit() if missing
	//   Create purchase row status=new, invoice_type=postpaid, amount, pricing_tier, extend_key_id, promo_code_id, idempotency_key
	//   RecordSaleTx with idempotency_key = "postpaid-sale:" + purchaseUUID or header key string
	//   Commit
	// ProcessPurchaseById(ctx, purchaseId)
	// On Process failure after commit: do NOT auto-reverse AR in v1 (log CRITICAL); return error
	// Prefer: if Process fails and purchase not paid, leave AR sale (ops adjust) OR cancel purchase + reverse sale in same recovery — **v1: if Process fails, reverse sale+cancel purchase in a compensating tx** to avoid orphan debt without keys
}
```

**Compensating failure policy (locked for plan):** If `ProcessPurchaseById` fails after sale commit, run compensating tx: cancel purchase if not paid, insert adjustment decrease (or reverse sale settlement-style) with idempotency `postpaid-sale-reverse:{purchaseID}` so balance_owed returns. Prefer explicit reverse helper `ReverseSaleTx` that only works for unpaid cancelled purchases.

Simpler v1 alternative (choose this): **single transaction cannot include Remnawave HTTP**. So:
1. Credit check + create purchase (status new) + sale ledger in one DB tx
2. ProcessPurchaseById
3. If process fails: compensating DB tx decreases owed (adjustment decrease) + cancel purchase

- [ ] **Step 1: Failing test** — inject fake credit repo + purchase repo if seams exist; otherwise unit-test switch rejects unknown without postpaid, then add postpaid path test with interfaces.

Minimal test without full payment fake:

```go
func TestCreatePurchaseWithOptionalExtendRejectsPostpaidWithoutRepo(t *testing.T) {
	// PaymentService with nil resellerCreditRepo
	// call create path with InvoiceTypePostpaid → error contains "not configured" or similar
}
```

Also add test that `parsePaymentMethod` is API-side (Task 6).

- [ ] **Step 2: Implement createPostpaidPurchase + switch case**

```go
case database.InvoiceTypePostpaid:
	return s.createPostpaidPurchase(ctx, amount, days, trafficLimitGB, customer, promoID, extendKeyID)
```

In `resumeExistingPurchase`, for postpaid: if paid return success; if new/pending/processing call ProcessPurchaseById like wallet.

- [ ] **Step 3: Wire constructor**

Find `NewPaymentService` / `cmd/app` wiring; pass `database.NewResellerCreditRepository(pool)`.

- [ ] **Step 4: Run tests**

```bash
go test ./internal/payment/ -count=1
go build ./cmd/app
```

- [ ] **Step 5: Commit**

```bash
git add internal/payment/ cmd/app/
git commit -m "feat(payment): postpaid purchase create with AR sale ledger"
```

---

### Task 6: API — parsePaymentMethod + CreatePurchase postpaid + reseller account APIs

**Files:**
- Modify: `internal/api/handlers.go` (`parsePaymentMethod`, `CreatePurchase` response for postpaid, new handlers)
- Modify: `internal/api/server.go` (routes)
- Create: `internal/api/reseller_postpaid_test.go`

**Interfaces:**
- `parsePaymentMethod`: add `case "postpaid": return database.InvoiceTypePostpaid, nil`
- CreatePurchase: already loads customer + ResolvePlanPrice; postpaid uses same service path; response should not require mobile instructions; optional `pricing_tier` already returned
- New types:

```go
type ResellerAccountResponse struct {
	CreditLimit     float64 `json:"credit_limit"`
	BalanceOwed     float64 `json:"balance_owed"`
	RemainingCredit float64 `json:"remaining_credit"`
	IsReseller      bool    `json:"is_reseller"`
}

type ResellerLedgerItem struct {
	ID            int64   `json:"id"`
	EntryType     string  `json:"entry_type"`
	Direction     string  `json:"direction"`
	Amount        float64 `json:"amount"`
	PurchaseID    *int64  `json:"purchase_id,omitempty"`
	EffectiveAt   string  `json:"effective_at"`
	Note          string  `json:"note,omitempty"`
	CreatedBy     string  `json:"created_by"`
}

type CreateSettlementRequest struct {
	Amount         float64 `json:"amount"`
	PaymentMethod  string  `json:"payment_method"` // "wallet" only in v1
	IdempotencyKey string  `json:"idempotency_key,omitempty"` // or header
}
```

- Routes (withAuth):
  - `GET /api/reseller/account`
  - `GET /api/reseller/ledger?limit=&offset=`
  - `POST /api/reseller/settlements` — wallet only: BeginTx DeductBalanceTx + wallet_transaction + RecordSettlementTx; created_by=`customer:{telegram_id}`

- [ ] **Step 1: Failing API tests**

```go
func TestParsePaymentMethodPostpaid(t *testing.T) {
	got, err := parsePaymentMethod("postpaid")
	if err != nil || got != database.InvoiceTypePostpaid {
		t.Fatalf("got %q err %v", got, err)
	}
}

func TestCreatePurchasePostpaidNonResellerRejected(t *testing.T) {
	// inject customer IsReseller=false, payment_method=postpaid → 400
}

func TestCreatePurchasePostpaidResellerCallsCreateWithPostpaidType(t *testing.T) {
	// inject createServicePurchase capturing invoiceType == postpaid and wholesale amount
}
```

- [ ] **Step 2: Implement handlers + routes**

Settlement wallet path (handler or thin service method on payment/reseller service):

```go
// amount must be >0 and <= balance_owed (repo enforces)
// DeductBalanceTx(customer, amount)
// walletTx type=purchase or refund? Use TypePurchase with description "Reseller AR settlement" OR add WalletTransactionTypeSettlement if CHECK allows — **prefer description on existing type `purchase` only if CHECK blocks new types**. Check wallet_transaction CHECK; if only topup|purchase|refund|referral, use `purchase` with clear description "AR settlement" and optional purchase_id null.
// RecordSettlementTx
```

If wallet CHECK is strict, do **not** add migration for wallet type unless needed — use `purchase` type with note.

- [ ] **Step 3: Run**

```bash
go test ./internal/api/ -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/api/
git commit -m "feat(api): postpaid purchase and reseller account/settlement endpoints"
```

---

### Task 7: Admin credit limit, settlements, ledger, list fields

**Files:**
- Modify: `internal/api/handlers.go` (`AdminCustomerByTelegramID` path router or split handlers)
- Modify: `internal/api/server.go`
- Modify: `internal/database/customer.go` `ListResellers` optional join — **prefer separate credit repo batch get** in handler to avoid bloating ListResellers SQL
- Create: `internal/api/admin_reseller_credit_test.go`

**Interfaces:**
- Extend `AdminResellerResponse`:

```go
type AdminResellerResponse struct {
	TelegramID      int64   `json:"telegram_id"`
	IsReseller      bool    `json:"is_reseller"`
	CreditLimit     float64 `json:"credit_limit"`
	BalanceOwed     float64 `json:"balance_owed"`
	RemainingCredit float64 `json:"remaining_credit"`
}
```

- `PATCH /api/admin/customers/{telegram_id}/credit` body `{"credit_limit": 50000}`
- `POST /api/admin/customers/{telegram_id}/settlements` body `{"amount": 1000, "note": "cash received"}` + Idempotency-Key header
- `GET /api/admin/customers/{telegram_id}/ledger`
- Path routing: expand `AdminCustomerByTelegramID` to switch on `parts[1]`:
  - `reseller` → existing PATCH flag
  - `credit` → PATCH limit
  - `settlements` → POST admin settlement (no wallet debit; created_by=`admin:{telegram_id}`)
  - `ledger` → GET

When enabling reseller (`is_reseller=true`), optionally `EnsureAccount(ctx, id, config.ResellerDefaultCreditLimit())`.

When disabling reseller with balance_owed > 0: allow flag clear; postpaid create still blocked by IsReseller check.

- [ ] **Step 1: Tests for set limit, admin settlement, list includes owed**

- [ ] **Step 2: Implement**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(api): admin reseller credit limit, settlements, and ledger"
```

---

### Task 8: Finance — settlement cash on settlement date

**Files:**
- Modify: `internal/database/purchase.go` revenue queries **only if** postpaid already counts as service revenue via `<> wallet_topup` (it will — no change needed for gross)
- Modify: `internal/reporting/service.go` and/or new `internal/database/reseller_credit.go` method `SumSettlementsInRange(ctx, from, to time.Time) (float64, error)`
- Modify: `BuildFinanceReport` / `FinanceService.GetReport` to add settlement totals into **CashCollected** (and not into gross again)
- Tests: `internal/reporting/*_test.go`

**Rule:**
- Gross service revenue: paid purchases including `postpaid` (already via `invoice_type <> 'wallet_topup'`)
- Cash collected: existing cash invoice types **plus** sum of `reseller_ledger_entry` where `entry_type='settlement'` and `effective_at` in range
- Do not add postpaid purchase amount to cash_collected

- [ ] **Step 1: Failing test** — BuildFinanceReport or service test with settlement rows increases CashCollected only

- [ ] **Step 2: Implement SumSettlements + wire into GetReport**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(reporting): count reseller AR settlements as cash collected"
```

---

### Task 9: Frontend Checkout postpaid option

**Files:**
- Modify: `web-app/src/pages/Checkout.tsx`
- Modify: `web-app/src/pages/Checkout.test.tsx`
- Modify: `web-app/src/lib/types.ts` (optional account types)
- Modify: `web-app/src/lib/translations.ts` (EN + MY keys)

**Behavior:**
- Extend `CheckoutAction = 'manual' | 'wallet' | 'topup' | 'postpaid'`
- On load for reseller, fetch `GET /api/reseller/account` (auth)
- Show Postpaid button when `is_reseller && remaining_credit >= targetAmount && credit_limit > 0`
- `createPurchase('postpaid')` sets `payment_method: 'postpaid'`, no promo
- Success: navigate home or show fulfilled state (wallet-like success; no screenshot UI)

- [ ] **Step 1: Failing vitest** — reseller sees postpaid button; non-reseller does not; create sends payment_method postpaid

- [ ] **Step 2: Implement**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(web): checkout postpaid option for resellers"
```

---

### Task 10: Reseller Account page + Home card

**Files:**
- Create: `web-app/src/pages/ResellerAccount.tsx`
- Create: `web-app/src/pages/ResellerAccount.test.tsx`
- Modify: `web-app/src/App.tsx` — route `/reseller/account`
- Modify: `web-app/src/pages/Home.tsx` — card when `is_reseller` (not admin-only)
- Modify: `web-app/src/lib/translations.ts`

**UI:**
- Show credit_limit, balance_owed, remaining_credit
- Ledger list from GET `/api/reseller/ledger`
- “Pay balance” → wallet settlement POST `/api/reseller/settlements` with amount (full or partial input), Idempotency-Key
- Gate: non-reseller → message / redirect home

- [ ] **Step 1: Tests** — renders balances; pay calls settlements API

- [ ] **Step 2: Implement**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(web): reseller account page and home card"
```

---

### Task 11: AdminResellers credit UI

**Files:**
- Modify: `web-app/src/pages/AdminResellers.tsx`
- Modify: `web-app/src/pages/AdminResellers.test.tsx`
- Modify: translations

**UI:**
- List shows limit / owed / remaining
- Set credit limit form (PATCH credit)
- Record offline settlement (amount + note)
- Link or expand ledger (GET admin ledger)

- [ ] **Step 1: Tests** — set limit PATCH body; settlement POST

- [ ] **Step 2: Implement**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat(web): admin reseller credit limit and settlement UI"
```

---

### Task 12: Docs

**Files:**
- Modify: `HOWTOUSE.md`
- Modify: `docs/MINI_APP.md`
- Modify: `readme.md` (API table rows only)

**Content (required topics):**
- Enable reseller + set credit limit
- Postpaid checkout behavior (immediate fulfill, increases owed)
- Settlement: reseller wallet pay-down + admin offline
- Finance: service revenue on sale; cash on settlement
- No negative wallet; no AR backfill; promo still blocked
- Env `RESELLER_DEFAULT_CREDIT_LIMIT`

- [ ] **Step 1: Write docs**

- [ ] **Step 2: Commit**

```bash
git commit -m "docs: reseller postpaid credit and sales ledger ops"
```

---

### Task 13: Full verification gate

**Files:** none (commands only)

- [ ] **Step 1: Backend**

```bash
go test ./...
go vet ./...
go build ./cmd/app
```

Expected: all PASS

- [ ] **Step 2: Frontend**

```bash
cd web-app && npm ci && npm test && npm run build
```

Expected: all PASS

- [ ] **Step 3: Money-safety diff check**

```bash
git diff main -- internal/wallet | wc -c
# wallet package should be empty or only if DeductBalanceTx called from new settlement path without changing DeductBalanceTx itself
```

Prefer settlement calling existing `DeductBalanceTx` without editing wallet package invariants.

- [ ] **Step 4: Confirm constraints**

- Crypto Pay still disabled in `parsePaymentMethod`
- No gift/assign
- Postpaid not in referral conversion

- [ ] **Step 5: Commit only if verification fixes needed; else stop**

---

## Self-review

### Spec coverage

| Spec item | Task |
|-----------|------|
| Migration 000035 AR tables + postpaid invoice | 1 |
| InvoiceType constant | 2 |
| Credit/ledger repo + invariants | 3 |
| Default credit limit config | 4 |
| Postpaid create + fulfill + sale ledger | 5 |
| API purchase postpaid + reseller account/ledger/settlements | 6 |
| Admin limit/settlement/ledger/list fields | 7 |
| Finance settlement cash | 8 |
| Checkout postpaid UI | 9 |
| Reseller account page + Home | 10 |
| Admin UI extensions | 11 |
| Docs | 12 |
| Verification | 13 |
| Bot postpaid | Explicitly deferred |
| Mobile banking settlement | Explicitly deferred |
| No AR backfill | Migration defaults only |

### Placeholder scan

No TBD/TODO steps; open design choices locked in Global Constraints.

### Type consistency

- `InvoiceTypePostpaid` = `"postpaid"` matches payment_method JSON and CHECK
- Ledger `direction` increase|decrease used by sale/settlement/adjustment
- AdminResellerResponse fields match frontend AdminResellers
- RemainingCredit = credit_limit - balance_owed computed server-side

---

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-12-reseller-postpaid-tracking.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — execute tasks in this session with checkpoints  

Which approach?
