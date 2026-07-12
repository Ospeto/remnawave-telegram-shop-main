# Reseller Wholesale Pricing — Design

**Date:** 2026-07-12  
**Status:** Approved for implementation planning  
**Branch target:** feature branch from `main` (not implemented yet)

## Problem

Resellers need to buy the same VPN plans at fixed wholesale rates (examples: retail 5000 → wholesale 4000; retail 15000 → wholesale 12000). Today the shop has a single list price per plan, percent-only promo codes, Telegram-only auth, and purchases always fulfill keys to the buyer. There is no reseller role, fixed wholesale price, or role-based pricing path.

## Goals

- Admin-approved Telegram users pay fixed wholesale prices per plan.
- Same Telegram Mini App purchase flow (no separate public website in v1).
- Keys remain on the reseller’s Telegram account (offline resale / sharing).
- Server-authoritative pricing; no client-trusted amounts.
- No promo stacking on wholesale.
- Preserve money-safety invariants (idempotency, wallet rules, fulfillment).

## Non-goals (v1)

- Standalone public webpage or non-Telegram auth
- Buy-for-another-user / gift / transfer of keys
- Per-reseller custom price lists
- Monthly quantity caps or prepaid inventory credits
- Crypto Pay re-enable
- Finance UI split of retail vs wholesale (store audit field only)
- Changing payment fulfillment or wallet balance mutation logic beyond amount selection

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Who gets the key | Buyer (reseller’s Telegram account) |
| Who is a reseller | Admin-approved customer flag |
| Where they buy | Same Mini App; automatic wholesale prices |
| Price model | Fixed `wholesale_price` per plan |
| Payment methods | Mobile banking + wallet (same as retail) |
| Promo codes | No stacking with wholesale |
| Purchase limits | None in v1 |
| Price display | Wholesale (effective) price only for resellers |
| Implementation approach | Role + optional `wholesale_price` on plans |

## Product model

### Reseller identity

- `customers.is_reseller boolean NOT NULL DEFAULT false`.
- Only admin can set/clear the flag.
- Non-resellers never receive wholesale pricing.

### Plans

- Existing plan catalog (`config.Plan` / `plans_catalog` in `app_config`) gains optional `wholesale_price`.
- Retail `price` remains the public list price.
- If `wholesale_price` is set: must be an integer `> 0` and `<= price` (admin save rejected otherwise).
- If reseller buys a plan with **no** wholesale price configured: **fall back to retail** (do not block sale).

### Price resolution

Single shared function used by Mini App purchase, Telegram bot sell path, and wallet service-buy path:

```
ResolvePlanPrice(plan, customer) → (amount int, pricingTier string)
  if customer.IsReseller && plan has wholesale_price:
    return wholesale_price, "wholesale"
  return plan.price, "retail"
```

- Client cannot supply service purchase amount.
- Wallet top-up is never wholesale and never promo-discounted.

### Promo rules

- If `customer.is_reseller`: reject `promo_code` on purchase create and on promo validate (clear 400 message).
- Non-resellers keep existing percent promo behavior.

### Fulfillment

- Unchanged: new key or extend owned key on the purchasing customer.
- No gift/assign in v1.

## Data model

### Migration(s)

1. `customers.is_reseller BOOLEAN NOT NULL DEFAULT FALSE`
2. `purchases.pricing_tier TEXT NOT NULL DEFAULT 'retail'`  
   - Allowed values: `retail`, `wholesale`  
   - Set at purchase create from `ResolvePlanPrice`  
   - Historical rows default `retail`

### Plan catalog JSON

Extend `config.Plan`:

- `WholesalePrice *int` (JSON `wholesale_price`, omit/null = unset)

Admin plan create/update accepts and persists this field via existing catalog save path.

### Purchase audit

- `amount` = charged amount (as today)
- `pricing_tier` = `retail` | `wholesale`
- Finance reporting continues to use paid amounts; tier enables later retail/wholesale volume splits without v1 Finance page work

## API

### Session / me

- Authenticated identity payload includes `is_reseller: boolean` so Mini App can render prices without guessing.

### `GET /api/plans`

- **Public / non-reseller session:** retail prices only; never expose `wholesale_price`.
- **Authenticated reseller:** each plan’s `price` field is the **effective** charge amount (wholesale if set, else retail). Optional `pricing_tier` per plan for UI badge.
- Do not return both retail and wholesale to non-resellers.

### `POST /api/purchase`

- Request shape unchanged (`plan_id` / `plan_index`, optional `extend_key_id`, `promo_code`, `payment_method`).
- Server resolves amount via `ResolvePlanPrice`.
- Reseller + promo → `400` with message that reseller pricing cannot combine with promos.
- Response includes charged amount and `pricing_tier`.

### Telegram bot

- Sell/create purchase path must call the same resolver so bot and Mini App stay consistent.

### Admin

- Toggle reseller: e.g. `PATCH /api/admin/customers/{telegram_id}/reseller` body `{ "is_reseller": true|false }` (admin auth).
- List resellers: simple admin list endpoint or filter on existing customer admin surface if present.
- Plan admin create/update: `wholesale_price` optional; validate `> 0` and `<= price`; null/omit clears wholesale.

## Mini App UX

### Plans / Checkout

- If `is_reseller`: show effective price only; small “Reseller price” label when tier is wholesale.
- No retail strikethrough in v1.
- Promo entry: hide or disable for resellers; server still enforces.

### Admin Mini App

- Plans editor: wholesale price field per plan.
- Reseller management: search by Telegram ID → toggle reseller on/off (minimal admin section/card).
- No separate `/reseller` storefront route.

## Error handling

| Case | Behavior |
|------|----------|
| Non-reseller forces wholesale | Impossible; server charges retail |
| Reseller + promo | 400 — reseller pricing cannot combine with promos |
| Missing/inactive plan | Existing behavior |
| Invalid wholesale on admin save | Reject save |
| Bad/missing wholesale at purchase | Fall back to retail |
| Reseller flag removed after pending create | Pending purchase keeps amount frozen at create; new purchases use current flag |

## Money safety

- Do not refactor payment fulfillment or wallet balance mutation beyond selecting amount/tier at create.
- Preserve idempotency keys, transaction-ID uniqueness, and wallet invariants.
- Top-ups remain full amount, no wholesale, no promo.
- Crypto Pay remains disabled.

## Testing

Minimum coverage:

1. `ResolvePlanPrice`: non-reseller retail; reseller+wholesale; reseller without wholesale → retail
2. Purchase create: reseller charged wholesale; promo rejected; non-reseller retail + promo still works
3. `GET /api/plans`: public/non-reseller never see wholesale; reseller sees effective price
4. Admin: toggle reseller; wholesale validation (`> retail`, `≤ 0`, clear)
5. Bot path uses same resolver (at least one test)
6. Existing payment/wallet tests remain green; no accidental money-path behavior change

## Rollout

1. Migrations (`is_reseller`, `pricing_tier`)
2. Plan catalog `wholesale_price` + admin plans UI
3. `ResolvePlanPrice` wired into all service purchase creates
4. `/api/me` + `/api/plans` + purchase response fields
5. Mini App Plans/Checkout display
6. Admin reseller toggle UI
7. Docs: HOWTOUSE + MINI_APP — approve resellers, set wholesale prices, no promo stacking
8. No historical purchase backfill as wholesale

## Future (explicitly later)

- Gift/assign purchase to another Telegram user
- Per-reseller price overrides
- Quantity caps / credit packs
- Public reseller portal with non-Telegram auth
- Finance page retail vs wholesale breakdown using `pricing_tier`

## Summary

v1 is a **role-gated fixed wholesale price** on the existing Mini App and purchase pipeline: admin marks resellers, admin sets per-plan wholesale prices, server resolves charge amount, promos do not stack, keys stay on the reseller account, and money-safety paths stay otherwise unchanged.
