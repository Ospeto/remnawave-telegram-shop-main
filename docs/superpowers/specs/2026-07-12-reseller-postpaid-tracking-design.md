# Reseller Postpaid Credit + Sales Ledger — Design

**Date:** 2026-07-12  
**Status:** Approved for implementation planning  
**Depends on:** Reseller wholesale pricing (`is_reseller`, `wholesale_price`, `pricing_tier`, `ResolvePlanPrice`)  
**Branch target:** feature branch from `main`

## Problem

Approved resellers can already buy at wholesale, but only prepaid (mobile banking or wallet). Operators need:

1. **Postpaid credit (reseller → shop):** resellers take stock / activate keys now and settle later.
2. **Sales ledger tracking:** admin and each reseller see that reseller’s wholesale purchases, amounts, and unpaid balance.

There is no AR/credit model today. Wallet is prepaid and non-negative only. Referral is a separate invite-bonus graph, not reseller channel tracking. Keys stay on the buyer; there is no end-customer downline.

## Goals

- Every approved reseller can optionally buy **on account** (postpaid) at checkout.
- Per-reseller **credit limit** with optional global default for new resellers.
- Fulfill **immediately** when remaining credit covers the order; otherwise block.
- **Separate AR ledger** (not negative wallet): sales increase owed; settlements decrease owed.
- Settlement via **reseller self-pay** and **admin offline** recording.
- Admin + that reseller see full own ledger, unpaid balance, limit, remaining credit.
- Server-authoritative amounts; preserve money-safety (idempotency, wallet invariants, fulfillment).
- Finance: service revenue on sale/fulfill date; cash collected on settlement date.

## Non-goals (v1)

- End-customer / downline / affiliate tracking
- Gift / assign keys to another user
- Negative wallet balance or reusing wallet as credit
- Crypto Pay re-enable
- Interest, late fees, auto-charge settlement
- Always-postpaid-only checkout (prepaid remains available)
- Historical backfill of past wholesale purchases into AR
- Automatic AR reverse on service refund (admin adjustment only if needed)
- Bot postpaid as required v1 (Mini App first; bot optional follow-up)

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Postpaid direction | Reseller owes the shop (AR) |
| Tracking | Sales ledger only (no downline) |
| Who can postpay | Every `is_reseller` automatically |
| Credit limits | Per-reseller limit + optional global default |
| Settlement | Self-pay + admin offline |
| Checkout | Optional: Pay now **or** Postpaid |
| Ledger visibility | Admin + that reseller (own data only) |
| Fulfillment timing | Immediate if within remaining credit; else block |
| Architecture | Separate AR ledger (not wallet, not purchase-status-only) |
| Overpay settlement | Reject in v1 |
| Price on postpaid | Same `ResolvePlanPrice` (wholesale when set) |
| Promo on postpaid | Still blocked for resellers |

## Product model

### Who

- Only customers with `is_reseller = true` may choose postpaid.
- Non-resellers never see postpaid options or reseller account APIs.
- Removing reseller flag while balance is owed: keep account/ledger; **block new postpaid**; allow settlement.

### Price

- Postpaid uses existing `ResolvePlanPrice(plan, isReseller)` → amount + `pricing_tier`.
- Promo codes remain rejected for resellers on all purchase paths.

### Credit account (per reseller)

| Field | Meaning |
|-------|---------|
| `credit_limit` | Max outstanding AR; admin-set |
| `balance_owed` | Current unpaid AR |
| Remaining credit | `credit_limit - balance_owed` |

- New resellers may receive optional global default limit from config (`0` = no postpaid until admin sets a limit).
- Order amount must be `<= remaining credit` under row lock; no partial fulfill.

### Checkout

Reseller chooses:

1. **Pay now** — existing mobile banking / wallet (unchanged).
2. **Postpaid** — credit check → create service purchase at resolved amount → **fulfill immediately** → ledger **sale** increases `balance_owed`.

No cash or wallet movement on postpaid create.

### Settlement

- **Self-pay:** reseller pays down owed balance (mobile banking receipt flow and/or wallet debit of settlement amount — exact rails fixed in implementation plan; must be server-authoritative and idempotent).
- **Admin offline:** admin records amount received + note → same settlement math.
- Partial settlement allowed; amount must be `> 0` and `<= balance_owed` (reject overpay in v1).
- Settlement never mutates purchase fulfillment; it only reduces AR.

### Tracking (sales ledger)

Chronological entries:

| Type | Effect on `balance_owed` | Links |
|------|--------------------------|--------|
| `sale` | +amount | Required `purchase_id` |
| `settlement` | −amount | Optional payment/ref note |
| `adjustment` | + or − via signed policy (v1: explicit direction + reason) | Admin only |

Surfaces:

- Reseller: unpaid total, limit, remaining, own ledger, pay-balance CTA.
- Admin: per-reseller limit/owed/remaining, set limit, record settlement, full ledger.

### Finance reporting

- **Gross service revenue:** include postpaid sales on **sale/fulfill date** (paid service purchase amounts).
- **Cash collected:** include settlements on **settlement effective date**; do not treat postpaid create as cash.
- Do not double-count when the reseller later settles.
- Wallet top-ups and prepaid paths unchanged.
- `financial_adjustment` refund ledger remains separate from AR.

## Data model

### Migration `000035` (next after `000034`)

**`reseller_credit_account`**

- `customer_id` PK/FK → `customer`
- `credit_limit` NUMERIC(18,2) NOT NULL DEFAULT 0
- `balance_owed` NUMERIC(18,2) NOT NULL DEFAULT 0
- `created_at`, `updated_at`
- CHECK `credit_limit >= 0`, `balance_owed >= 0`
- Created lazily on first postpaid use or when admin sets limit

**`reseller_ledger_entry`**

- `id` bigserial/uuid PK
- `customer_id` FK → `customer`
- `entry_type` TEXT CHECK IN (`sale`, `settlement`, `adjustment`)
- `amount` NUMERIC(18,2) NOT NULL CHECK `amount > 0`
- Direction: sale increases owed; settlement decreases owed; adjustment uses explicit `direction` or signed convention documented in plan (prefer explicit `direction` enum `increase|decrease` for adjustments)
- `purchase_id` nullable FK → `purchase` (required when `entry_type = sale`)
- `effective_at` timestamptz NOT NULL
- `note` text nullable
- `created_by` (admin telegram id or system marker)
- `idempotency_key` TEXT NOT NULL UNIQUE
- `created_at`
- No hard-delete of history; corrections are new rows

### Purchase changes (minimal)

- Add invoice type **`postpaid`** to purchase `invoice_type` CHECK (today: `crypto`, `mobile_banking`, `wallet_topup`, `wallet_payment`, `yookasa`).
- Amount from `ResolvePlanPrice`; `pricing_tier` stamped as today.
- Postpaid create: fulfill service purchase **without** wallet debit or mobile banking receipt verification.
- Must not use negative wallet balance.

### Invariants

1. Postpaid create and settlement update `balance_owed` in the **same DB transaction** as the ledger insert.
2. Create: lock account row; require `balance_owed + amount <= credit_limit`.
3. Settlement: lock account row; require `amount <= balance_owed`; never negative owed.
4. Idempotency keys on sale and settlement paths; replay returns same result without double debt/pay.
5. Wallet top-up / wallet spend / referral bonus paths unchanged.
6. Crypto Pay stays disabled.
7. Service refund ledger (`financial_adjustment`) ≠ AR settlement.

### Config

- Optional app/env: `RESELLER_DEFAULT_CREDIT_LIMIT` (numeric; `0` = no credit until admin sets).

## API surfaces

All money paths server-authoritative; existing admin/auth middleware patterns.

| Endpoint | Auth | Purpose |
|----------|------|---------|
| `POST /api/purchase` | reseller | Accept postpaid payment method / invoice type; resolve price; credit check; fulfill + ledger sale |
| `GET /api/reseller/account` | that reseller | `credit_limit`, `balance_owed`, `remaining_credit` |
| `GET /api/reseller/ledger` | that reseller | Own paginated ledger |
| `POST /api/reseller/settlements` | that reseller | Self-pay settlement (idempotent) |
| `GET /api/admin/resellers` | admin | Extend with limit / owed / remaining |
| `PATCH /api/admin/customers/{telegram_id}/credit` | admin | Set `credit_limit` |
| `POST /api/admin/customers/{telegram_id}/settlements` | admin | Offline settlement + note |
| `GET /api/admin/customers/{telegram_id}/ledger` | admin | Full ledger for one reseller |

### Errors

| Case | Response |
|------|----------|
| Non-reseller postpaid | 400/403 |
| `credit_limit = 0` or insufficient remaining | 400 clear message |
| Over-settlement | 400 |
| Unauth / non-owner ledger | existing 401/403 patterns |
| Invalid amount | 400 |

## UI surfaces

### Mini App (reseller)

- **Checkout:** for `is_reseller`, show **Postpaid** alongside mobile banking / wallet when remaining credit allows (or limit &gt; 0 with messaging).
- **Reseller Account** page (e.g. `/reseller/account`): unpaid balance, limit, remaining, ledger list, “Pay balance” CTA.
- **Home:** card for resellers → own account/ledger (not admin-only).

### Mini App (admin)

- **Admin Resellers:** show limit / owed / remaining; edit limit; record settlement; open ledger.
- No public leak of other resellers’ balances.

### Telegram bot

- v1: **Mini App first** for postpaid. Bot postpaid button is optional follow-up, not required for launch.

## Error / edge cases

- Concurrent postpaid buys: account row lock so two orders cannot both pass the same remaining credit.
- Settlement races: same lock; never negative `balance_owed`.
- Idempotent replay of sale/settlement keys: no double debt/pay.
- Reseller flag cleared with outstanding balance: settle allowed; new postpaid blocked.
- Refund of a postpaid sale: no auto AR reverse in v1; admin **adjustment** only if needed.

## Testing requirements

- Postpaid still uses wholesale via `ResolvePlanPrice` when configured.
- Create within limit → fulfill + `balance_owed += amount` + sale ledger row.
- Create over limit → reject; no fulfill; no ledger; balance unchanged.
- Partial settlement reduces owed; overpay rejected.
- Admin offline settlement same math as self-pay.
- Non-reseller cannot postpaid.
- Reseller cannot read another reseller’s ledger.
- Wallet prepaid paths unchanged (no negative wallet).
- Finance: sale-date service revenue vs settlement-date cash (as practical in unit/integration).
- Frontend: checkout option, account page, admin limit/settlement.
- Idempotency replay tests for sale and settlement.

## Rollout order

1. Migration `000035` + credit account / ledger repository + invariants  
2. Admin set credit limit + list fields  
3. Postpaid purchase path (create + fulfill + sale ledger)  
4. Settlement (admin offline + reseller self-pay)  
5. Reseller account UI + admin resellers extensions  
6. Finance reporting hooks for postpaid sale vs settlement cash  
7. Docs (`HOWTOUSE.md`, `docs/MINI_APP.md`)  
8. No AR backfill of historical prepaid wholesale purchases  

## Hard constraints

- Do **not** turn wallet into credit or allow negative wallet balance.
- Do **not** re-enable Crypto Pay.
- Do **not** gift/assign keys or build downline tracking in v1.
- Money paths stay idempotent; `balance_owed` updates are transactional with ledger inserts.
- Do not casually refactor payment fulfillment or wallet mutation beyond postpaid amount/tier selection and AR side effects.
- Server-authoritative price and settlement amounts; no client-trusted balances.

## Approach rejected (record)

| Approach | Why rejected |
|----------|----------------|
| Negative wallet as credit | Breaks prepaid wallet invariants; mixes top-ups/referral with AR |
| Purchase status-only “on_account” without ledger | Weak partial settlement; messy reporting; no clean self-pay |

## Open implementation details (for writing-plans)

These do not change product decisions; plan must pick one concrete option each:

1. Self-pay rails: mobile banking settlement invoice vs wallet debit of owed amount vs both.
2. Adjustment row shape: explicit `direction` column vs signed amount.
3. Whether bot postpaid is in the same plan or a follow-up task marked optional.

---

**Approval:** Product §1–§4 approved in design dialogue (2026-07-12). Ready for implementation plan via writing-plans skill after user reviews this file.
