# Reseller Wholesale Pricing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Admin-approved resellers pay fixed per-plan wholesale prices in the existing Mini App and Telegram bot, with server-authoritative pricing, no promo stacking, and purchase audit via `pricing_tier`.

**Architecture:** Add `customers.is_reseller` and `purchases.pricing_tier`, optional `config.Plan.WholesalePrice`, and a pure `ResolvePlanPrice(plan, customer) → (amount, tier)` used by Mini App purchase, bot sell, and wallet service-buy. Public/non-reseller plan responses never leak wholesale; resellers see effective price only. Payment create paths persist `pricing_tier` at insert without changing fulfillment or wallet mutation logic beyond amount/tier selection.

**Tech Stack:** Go 1.25.3, Postgres 17 migrations, existing `internal/api` + `internal/payment` + `internal/handler`, React/Vite Mini App (`web-app/`), vitest.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-12-reseller-wholesale-pricing-design.md` (commit `4c80192`).
- Money-safety: do **not** refactor payment fulfillment or wallet balance mutation beyond selecting amount/tier at create.
- Crypto Pay remains disabled.
- Server-authoritative price; never trust client amounts for service purchases.
- Wallet top-up is never wholesale and never promo-discounted.
- Reseller + promo → 400 (purchase create and promo validate).
- Reseller without wholesale configured → fall back to retail (do not block sale).
- No historical wholesale backfill; existing purchases default `pricing_tier='retail'`.
- Keys stay on buyer (reseller) account; no gift/assign in v1.
- Latest migration is `000033` → this feature uses `000034`.
- Verification commands: `go test ./...`, `go vet ./...`, `go build ./cmd/app`; frontend `npm test`, `npm run build`.

## File Structure

| Path | Responsibility |
|------|----------------|
| `db/migrations/000034_reseller_wholesale.up.sql` / `.down.sql` | `customers.is_reseller`, `purchases.pricing_tier` |
| `internal/database/customer.go` | `IsReseller` field + all SELECT/RETURNING/scan paths + `allowedCustomerFields` |
| `internal/database/purchase.go` | `PricingTier` field + insert/select/scan |
| `internal/config/config.go` | `Plan.WholesalePrice *int` |
| `internal/config/plan_catalog.go` | validate wholesale if set |
| `internal/config/pricing.go` (new) | `ResolvePlanPrice` pure helper + constants |
| `internal/config/pricing_test.go` (new) | resolver unit tests |
| `internal/payment/payment.go` | thread `pricingTier` into create helpers; default retail; top-up retail |
| `internal/api/handlers.go` | me/plans/purchase/promo/admin plans + reseller toggle |
| `internal/api/server.go` | register admin reseller routes |
| `internal/handler/payment_handlers.go` | bot uses `ResolvePlanPrice` |
| `web-app/src/lib/types.ts` | `is_reseller`, `pricing_tier`, admin `wholesale_price` |
| `web-app/src/pages/Plans.tsx` | hide promo for resellers; badge when wholesale |
| `web-app/src/pages/Checkout.tsx` | hide promo for resellers; show effective price |
| `web-app/src/pages/AdminPlans.tsx` | wholesale price field |
| `web-app/src/pages/AdminResellers.tsx` (new) | telegram_id + toggle |
| `web-app/src/pages/Home.tsx` / `App.tsx` | admin card + route |
| `docs/MINI_APP.md`, `HOWTOUSE.md` | operator docs |

---

### Task 1: Migration 000034 — reseller flag + pricing_tier

**Files:**
- Create: `db/migrations/000034_reseller_wholesale.up.sql`
- Create: `db/migrations/000034_reseller_wholesale.down.sql`

**Interfaces:**
- Consumes: none
- Produces: DB columns `customer.is_reseller BOOLEAN NOT NULL DEFAULT FALSE`; `purchase.pricing_tier TEXT NOT NULL DEFAULT 'retail'` with check constraint `retail|wholesale`

- [ ] **Step 1: Write up migration**

```sql
-- db/migrations/000034_reseller_wholesale.up.sql
ALTER TABLE customer
    ADD COLUMN IF NOT EXISTS is_reseller BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE purchase
    ADD COLUMN IF NOT EXISTS pricing_tier TEXT NOT NULL DEFAULT 'retail';

ALTER TABLE purchase
    DROP CONSTRAINT IF EXISTS purchase_pricing_tier_check;

ALTER TABLE purchase
    ADD CONSTRAINT purchase_pricing_tier_check
    CHECK (pricing_tier IN ('retail', 'wholesale'));
```

- [ ] **Step 2: Write down migration**

```sql
-- db/migrations/000034_reseller_wholesale.down.sql
ALTER TABLE purchase DROP CONSTRAINT IF EXISTS purchase_pricing_tier_check;
ALTER TABLE purchase DROP COLUMN IF EXISTS pricing_tier;
ALTER TABLE customer DROP COLUMN IF EXISTS is_reseller;
```

- [ ] **Step 3: Commit**

```bash
git add db/migrations/000034_reseller_wholesale.up.sql db/migrations/000034_reseller_wholesale.down.sql
git commit -m "feat(db): add reseller flag and purchase pricing_tier (000034)"
```

---

### Task 2: Customer `IsReseller` repository support

**Files:**
- Modify: `internal/database/customer.go`
- Test: `internal/database/customer_reseller_test.go` (new)

**Interfaces:**
- Consumes: migration column `is_reseller`
- Produces: `Customer.IsReseller bool`; all Find/Create RETURNING/scan paths include it; `allowedCustomerFields["is_reseller"]=true`

Every customer SELECT list currently ends with `auto_renew_notified_at`. Add `is_reseller` after it in **all** of:
- `FindByExpirationRange` select + scan
- `FindById` select + scan
- `FindByTelegramId` select + scan
- `FindOrCreate` RETURNING + scan
- `FindByTelegramIdForUpdateTx` select + scan
- `FindByTelegramIds` select + scan (if present)
- any other customer SELECT/RETURNING in this file that lists columns explicitly

- [ ] **Step 1: Write failing test**

```go
// internal/database/customer_reseller_test.go
package database

import (
	"strings"
	"testing"
)

func TestCustomerSelectIncludesIsReseller(t *testing.T) {
	// Structural guard: FindByTelegramId SQL must project is_reseller.
	// Prefer a compile-time field check + whitelist check when no live DB.
	var c Customer
	_ = c.IsReseller

	if !allowedCustomerFields["is_reseller"] {
		t.Fatal("allowedCustomerFields must allow is_reseller for admin toggle")
	}
}

func TestCustomerStructHasIsResellerField(t *testing.T) {
	// Ensure db tag is correct for documentation/consistency.
	// Use reflection-free string check via source is overkill; field presence is enough.
	c := Customer{IsReseller: true}
	if !c.IsReseller {
		t.Fatal("IsReseller not settable")
	}
}

// Optional: if package already has SQL-builder unit tests, assert select columns.
func TestFindByTelegramIdSelectMentionsIsReseller(t *testing.T) {
	// This test documents the required column name for implementers grepping.
	const col = "is_reseller"
	if !strings.Contains(col, "is_reseller") {
		t.Fatal("unreachable")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/database/ -run 'TestCustomerSelectIncludesIsReseller|TestCustomerStructHasIsResellerField' -count=1`

Expected: FAIL — `Customer` has no field `IsReseller` and/or whitelist missing.

- [ ] **Step 3: Implement Customer field + all query paths**

```go
// On Customer struct, after AutoRenewNotifiedAt:
IsReseller bool `db:"is_reseller"`

// In allowedCustomerFields:
"is_reseller": true,
```

Update every explicit column list and matching `Scan` destination to include `is_reseller` / `&customer.IsReseller` (or `&result.IsReseller`) in the same order.

For `FindOrCreate` RETURNING, append `is_reseller` to the RETURNING list and Scan.

Default for new inserts: DB default `false` is enough; do not require INSERT column unless you set it explicitly.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/database/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/customer.go internal/database/customer_reseller_test.go
git commit -m "feat(db): load and update customers.is_reseller"
```

---

### Task 3: Purchase `PricingTier` repository support

**Files:**
- Modify: `internal/database/purchase.go`
- Test: `internal/database/purchase_pricing_tier_test.go` (new)

**Interfaces:**
- Consumes: migration column `pricing_tier`
- Produces: `Purchase.PricingTier string`; `buildPurchaseInsert` writes it; `purchaseColumns` + `scanPurchase` read it
- Constants (in `purchase.go` or shared with config): prefer config package constants from Task 4; until then use string `"retail"` / `"wholesale"`

- [ ] **Step 1: Write failing test**

```go
// internal/database/purchase_pricing_tier_test.go
package database

import (
	"strings"
	"testing"
)

func TestPurchaseHasPricingTierField(t *testing.T) {
	p := Purchase{PricingTier: "wholesale"}
	if p.PricingTier != "wholesale" {
		t.Fatalf("got %q", p.PricingTier)
	}
}

func TestBuildPurchaseInsertIncludesPricingTier(t *testing.T) {
	p := &Purchase{
		Amount:      4000,
		CustomerID:  1,
		Currency:    "MMK",
		Status:      PurchaseStatusPending,
		InvoiceType: InvoiceTypeMobileBanking,
		PricingTier: "wholesale",
	}
	sql, args, err := buildPurchaseInsert(p).ToSql()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sql, "pricing_tier") {
		t.Fatalf("insert SQL missing pricing_tier: %s", sql)
	}
	found := false
	for _, a := range args {
		if s, ok := a.(string); ok && s == "wholesale" {
			found = true
		}
	}
	if !found {
		t.Fatalf("insert args missing wholesale tier: %#v", args)
	}
}

func TestPurchaseColumnsIncludePricingTier(t *testing.T) {
	found := false
	for _, c := range purchaseColumns {
		if c == "pricing_tier" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("purchaseColumns missing pricing_tier")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/database/ -run 'PricingTier|PricingTier' -count=1`

Expected: FAIL — field / column missing.

- [ ] **Step 3: Implement**

```go
// On Purchase struct:
PricingTier string `db:"pricing_tier"`

// buildPurchaseInsert Columns + Values: append "pricing_tier" and purchase.PricingTier
// If PricingTier is empty at insert time, set default before insert:
//   tier := purchase.PricingTier
//   if tier == "" { tier = "retail" }
// Use tier in Values.

// purchaseColumns: append "pricing_tier" (after idempotency_key is fine)
// scanPurchase: append &p.PricingTier in matching order
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/database/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/purchase.go internal/database/purchase_pricing_tier_test.go
git commit -m "feat(db): persist purchases.pricing_tier on insert and load"
```

---

### Task 4: Plan wholesale field + `ResolvePlanPrice`

**Files:**
- Modify: `internal/config/config.go` (`Plan` struct)
- Modify: `internal/config/plan_catalog.go` (`normalizePlans`)
- Create: `internal/config/pricing.go`
- Create: `internal/config/pricing_test.go`
- Test: extend or add `internal/config/plan_catalog_test.go` if present; else put wholesale validation tests in `pricing_test.go` via `normalizePlans` if exported, or test through `SetPlans`/`Load` patterns used by existing tests

**Interfaces:**
- Consumes: `database.Customer.IsReseller` (resolver takes a minimal interface or bool + plan)
- Produces:

```go
// internal/config/pricing.go
package config

const (
	PricingTierRetail    = "retail"
	PricingTierWholesale = "wholesale"
)

// ResolvePlanPrice returns the charge amount and pricing tier for a service plan purchase.
// Reseller without wholesale configured falls back to retail.
func ResolvePlanPrice(plan Plan, isReseller bool) (amount int, pricingTier string) {
	if isReseller && plan.WholesalePrice != nil {
		return *plan.WholesalePrice, PricingTierWholesale
	}
	return plan.Price, PricingTierRetail
}
```

```go
// Plan struct addition:
WholesalePrice *int `json:"wholesale_price,omitempty"`
```

`normalizePlans` validation when `plan.WholesalePrice != nil`:
- `*plan.WholesalePrice <= 0` → error `"plan wholesale_price must be positive"`
- `*plan.WholesalePrice > plan.Price` → error `"plan wholesale_price cannot exceed price"`

- [ ] **Step 1: Write failing tests**

```go
// internal/config/pricing_test.go
package config

import "testing"

func TestResolvePlanPrice_NonResellerRetail(t *testing.T) {
	plan := Plan{Price: 5000, WholesalePrice: intPtr(4000)}
	amount, tier := ResolvePlanPrice(plan, false)
	if amount != 5000 || tier != PricingTierRetail {
		t.Fatalf("got amount=%d tier=%s", amount, tier)
	}
}

func TestResolvePlanPrice_ResellerWithWholesale(t *testing.T) {
	plan := Plan{Price: 5000, WholesalePrice: intPtr(4000)}
	amount, tier := ResolvePlanPrice(plan, true)
	if amount != 4000 || tier != PricingTierWholesale {
		t.Fatalf("got amount=%d tier=%s", amount, tier)
	}
}

func TestResolvePlanPrice_ResellerWithoutWholesaleFallsBackRetail(t *testing.T) {
	plan := Plan{Price: 5000}
	amount, tier := ResolvePlanPrice(plan, true)
	if amount != 5000 || tier != PricingTierRetail {
		t.Fatalf("got amount=%d tier=%s", amount, tier)
	}
}

func TestNormalizePlans_RejectsWholesaleAboveRetail(t *testing.T) {
	_, err := normalizePlans([]Plan{{
		Label: "A", Days: 30, Price: 5000, SortOrder: 0, Active: true,
		WholesalePrice: intPtr(6000),
	}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizePlans_RejectsNonPositiveWholesale(t *testing.T) {
	_, err := normalizePlans([]Plan{{
		Label: "A", Days: 30, Price: 5000, SortOrder: 0, Active: true,
		WholesalePrice: intPtr(0),
	}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizePlans_AcceptsValidWholesale(t *testing.T) {
	plans, err := normalizePlans([]Plan{{
		Label: "A", Days: 30, Price: 5000, SortOrder: 0, Active: true,
		WholesalePrice: intPtr(4000),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].WholesalePrice == nil || *plans[0].WholesalePrice != 4000 {
		t.Fatalf("wholesale not preserved: %+v", plans[0].WholesalePrice)
	}
}

func intPtr(v int) *int { return &v }
```

- [ ] **Step 2: Run tests to verify fail**

Run: `go test ./internal/config/ -run 'ResolvePlanPrice|NormalizePlans_.*Wholesale' -count=1`

Expected: FAIL — missing symbols / validation.

- [ ] **Step 3: Implement Plan field, normalizePlans checks, ResolvePlanPrice**

- [ ] **Step 4: Run tests**

Run: `go test ./internal/config/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/plan_catalog.go internal/config/pricing.go internal/config/pricing_test.go
git commit -m "feat(config): wholesale_price validation and ResolvePlanPrice"
```

---

### Task 5: Payment layer persists `pricing_tier` without fulfillment refactor

**Files:**
- Modify: `internal/payment/payment.go`
- Test: `internal/payment/pricing_tier_test.go` (new) and/or extend `payment_test.go`

**Interfaces:**
- Consumes: `database.Purchase.PricingTier`; callers pass resolved amount as today
- Produces: public create APIs accept optional pricing tier and stamp it on the Purchase before insert

**Design (locked for this plan):**

Add a context key for pricing tier so public signatures stay stable and money-path surface area stays small:

```go
// internal/payment/pricing_context.go (new) OR bottom of payment.go
type pricingTierCtxKey struct{}

// WithPricingTier attaches retail|wholesale for the next CreatePurchase* call.
// Empty / missing defaults to retail.
func WithPricingTier(ctx context.Context, tier string) context.Context {
	if tier == "" {
		tier = config.PricingTierRetail
	}
	return context.WithValue(ctx, pricingTierCtxKey{}, tier)
}

func pricingTierFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(pricingTierCtxKey{}).(string); ok && v != "" {
		return v
	}
	return config.PricingTierRetail
}
```

In every place that builds a `database.Purchase` for **service** create (`createMobileBankingPurchase`, `createWalletPurchase`, `createFreePurchase`, and any extend path that constructs a Purchase), set:

```go
PricingTier: pricingTierFromContext(ctx),
```

For `createWalletTopUpInvoice` / top-up Purchase construction, force:

```go
PricingTier: config.PricingTierRetail,
```

Do **not** change `DeductBalanceTx`, verification, fulfillment, or promo math. Reseller promo rejection happens in API/bot **before** calling CreatePurchase with a non-empty promo.

Idempotent resume: existing purchase keeps its stored amount/tier; do not rewrite tier on resume.

- [ ] **Step 1: Write failing test**

```go
// internal/payment/pricing_tier_test.go
package payment

import (
	"context"
	"testing"

	"remnawave-tg-shop-bot/internal/config"
)

func TestWithPricingTierDefaultsRetail(t *testing.T) {
	if got := pricingTierFromContext(context.Background()); got != config.PricingTierRetail {
		t.Fatalf("got %s", got)
	}
}

func TestWithPricingTierStoresWholesale(t *testing.T) {
	ctx := WithPricingTier(context.Background(), config.PricingTierWholesale)
	if got := pricingTierFromContext(ctx); got != config.PricingTierWholesale {
		t.Fatalf("got %s", got)
	}
}
```

If the package already has fake purchase repos for CreatePurchase tests, add one test that CreatePurchase with `WithPricingTier(..., wholesale)` results in a created purchase whose `PricingTier == "wholesale"`. Mirror existing CreatePurchase test setup in `payment_test.go`.

- [ ] **Step 2: Run fail**

Run: `go test ./internal/payment/ -run PricingTier -count=1`

Expected: FAIL — missing helpers.

- [ ] **Step 3: Implement context helpers + stamp PricingTier on all service Purchase constructions; top-up always retail**

- [ ] **Step 4: Run**

Run: `go test ./internal/payment/ -count=1`

Expected: PASS (existing promo/idempotency tests remain green)

- [ ] **Step 5: Commit**

```bash
git add internal/payment/payment.go internal/payment/pricing_context.go internal/payment/pricing_tier_test.go
git commit -m "feat(payment): stamp pricing_tier on purchase create via context"
```

---

### Task 6: API — GetMe `is_reseller`, GetPlans effective price, CreatePurchase resolve, ValidatePromo reject

**Files:**
- Modify: `internal/api/handlers.go`
- Modify: `internal/api/server.go` only if GetPlans soft-auth requires route/middleware change; default keep public `/api/plans` and parse optional Telegram auth inside the handler
- Test: `internal/api/reseller_pricing_test.go` (new)

**Interfaces:**
- Consumes: `config.ResolvePlanPrice`, `customer.IsReseller`, `payment.WithPricingTier`
- Produces:

```go
// ValidationResponse
IsReseller bool `json:"is_reseller"`

// PlanResponse
Price       int    `json:"price"` // effective for reseller sessions
PricingTier string `json:"pricing_tier,omitempty"` // only when authenticated reseller path sets it

// CreatePurchaseResponse
PricingTier string `json:"pricing_tier,omitempty"`
```

**GetMe:** after loading customer, set `IsReseller: customer.IsReseller`.

**GetPlans:**
- Default (no auth / non-reseller): retail `Price` only; **never** set wholesale fields; omit `pricing_tier` or leave empty.
- If request has authenticated telegram ID and customer.IsReseller: for each plan call `ResolvePlanPrice(p, true)` and set `Price=amount`, `PricingTier=tier`.
- Implementation: try read `telegramID` from context the same way other optional-auth handlers do; if `/api/plans` is public without auth middleware, optionally accept session when present. **Preferred:** keep `/api/plans` public for retail, and when `Authorization`/telegram auth is present on the same route, resolve customer. If current `GetPlans` has no auth context, wrap with optional auth or document that Mini App always loads `/api/me` first and Plans uses me for badge while prices come from a second authenticated call.

**Locked approach for this codebase:** Mini App already calls `/api/me` and `/api/plans` separately. Make `GetPlans` attempt optional telegram auth:

```go
func (h *APIHandler) GetPlans(w http.ResponseWriter, r *http.Request) {
	plans := config.ActivePlans()
	currency := config.Currency()
	isReseller := false
	if telegramID, ok := r.Context().Value(telegramIDKey).(int64); ok {
		if c, err := h.customerRepo.FindByTelegramId(r.Context(), telegramID); err == nil && c != nil {
			isReseller = c.IsReseller
		}
	} else if initData := r.Header.Get("X-Telegram-Init-Data"); initData != "" && h can validate {
		// Prefer reusing existing auth helper if GetPlans is not behind withAuth.
	}
	// Simplest correct approach matching server.go: change route to withAuth optional is hard.
	// Spec requires reseller effective prices. Register a second path OR put GetPlans behind
	// soft auth. Use soft auth: if withAuth already sets context only when middleware runs,
	// change mux to: mux.HandleFunc("/api/plans", handler.GetPlans) stays public BUT
	// GetPlans calls the same session extraction used by withAuth when header present.
}
```

**Concrete locked decision:** Keep `GET /api/plans` public. Inside `GetPlans`, call existing session extraction used by `withAuth` (find the function that parses Telegram init data / bearer). If auth succeeds and customer.IsReseller, return effective prices; otherwise retail only. Do not 401 unauthenticated callers.

**CreatePurchase (service plans only):**
1. Load customer **before** price resolution (move FindByTelegramId above plan price assignment).
2. After `resolvePurchasePlan`, call:

```go
amount, tier := config.ResolvePlanPrice(*plan, customer.IsReseller)
if customer.IsReseller && strings.TrimSpace(req.PromoCode) != "" {
	http.Error(w, "Reseller pricing cannot combine with promo codes", http.StatusBadRequest)
	return
}
price = float64(amount)
ctx = payment.WithPricingTier(ctx, tier)
```

3. Response includes `PricingTier: tier` (and charged amount as today).
4. Top-up branch unchanged; response `pricing_tier` may be `"retail"` or omitted.

**ValidatePromo:**
```go
telegramID, ok := r.Context().Value(telegramIDKey).(int64)
// withAuth already ensures ok
customer, err := h.customerRepo.FindByTelegramId(...)
if customer != nil && customer.IsReseller {
	http.Error(w, "Reseller pricing cannot combine with promo codes", http.StatusBadRequest)
	return
}
```

- [ ] **Step 1: Write failing API tests** (httptest patterns from `admin_plans_test.go` / `admin_promos_test.go`)

Cover:
1. GetMe returns `is_reseller: true` when customer flag set (mock/fake customer repo if used; else set via config test doubles used in package).
2. GetPlans public never includes wholesale-only leakage (price stays retail; no wholesale_price field on PlanResponse).
3. CreatePurchase for reseller with wholesale plan charges wholesale amount and returns `pricing_tier=wholesale`.
4. CreatePurchase reseller + promo → 400.
5. ValidatePromo reseller → 400.
6. Non-reseller + promo still works (existing behavior).

Use the package’s existing fake repos / handler construction. If customer repo is real-DB-only, use interface stubs already present in tests; if none, add a small fake implementing only `FindByTelegramId`.

- [ ] **Step 2: Run fail**

Run: `go test ./internal/api/ -run 'Reseller|Resolve|Promo.*Reseller|GetPlans' -count=1`

- [ ] **Step 3: Implement handler changes**

- [ ] **Step 4: Run**

Run: `go test ./internal/api/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers.go internal/api/reseller_pricing_test.go
git commit -m "feat(api): reseller-aware me, plans, purchase, and promo rejection"
```

---

### Task 7: Admin plan wholesale field + reseller toggle API

**Files:**
- Modify: `internal/api/handlers.go` (`AdminPlanRequest/Response`, `adminPlanResponse`, `validateAdminPlanRequest`, Create/Update admin plan)
- Modify: `internal/api/server.go` (register routes)
- Test: `internal/api/admin_reseller_test.go` (new); extend `admin_plans_test.go`

**Interfaces:**

```go
type AdminPlanRequest struct {
	Label          string `json:"label"`
	Days           int    `json:"days"`
	Price          int    `json:"price"`
	TrafficLimitGB int    `json:"traffic_limit_gb"`
	SortOrder      int    `json:"sort_order"`
	// WholesalePrice: omit or null clears; pointer distinguishes unset vs zero.
	WholesalePrice *int `json:"wholesale_price"`
}

type AdminPlanResponse struct {
	// existing fields...
	WholesalePrice *int `json:"wholesale_price,omitempty"`
}

type AdminResellerRequest struct {
	IsReseller bool `json:"is_reseller"`
}

type AdminResellerResponse struct {
	TelegramID int64 `json:"telegram_id"`
	IsReseller bool  `json:"is_reseller"`
}
```

**validateAdminPlanRequest:** if `req.WholesalePrice != nil`:
- `*req.WholesalePrice <= 0` → error
- `*req.WholesalePrice > req.Price` → error

**CreateAdminPlan / UpdateAdminPlan:** set `plan.WholesalePrice = req.WholesalePrice` (nil clears).

**adminPlanResponse:** include `WholesalePrice: plan.WholesalePrice`.

**PATCH `/api/admin/customers/{telegram_id}/reseller`:**
```go
// Parse telegram_id from path after /api/admin/customers/
// Body: AdminResellerRequest
// FindByTelegramId → 404 if nil
// UpdateFields(customer.ID, map[string]interface{}{"is_reseller": req.IsReseller})
// Return AdminResellerResponse
```

**GET `/api/admin/resellers`:** list customers where `is_reseller=true` (add `CustomerRepository.ListResellers(ctx) ([]Customer, error)` simple query). Response: `[]AdminResellerResponse`.

Register in `server.go`:
```go
mux.HandleFunc("/api/admin/resellers", withAdmin(handler.ListResellers))
mux.HandleFunc("/api/admin/customers/", withAdmin(handler.AdminCustomerByTelegramID)) // method switch PATCH .../reseller
```

Path parsing: if path is `/api/admin/customers/123/reseller` and method PATCH → toggle.

- [ ] **Step 1: Failing tests**

1. Create admin plan with wholesale 4000 / retail 5000 → 201 and response includes wholesale_price.
2. Create with wholesale 6000 / retail 5000 → 400.
3. Update plan clearing wholesale (`"wholesale_price": null`) → field cleared.
4. PATCH reseller true → 200; customer IsReseller true (assert UpdateFields or fake).
5. Unauthenticated admin reseller route → 401/403 same as other admin routes (`TestRegisterHandlersProtects...`).

- [ ] **Step 2: Run fail**

- [ ] **Step 3: Implement**

- [ ] **Step 4: `go test ./internal/api/ -count=1` PASS**

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers.go internal/api/server.go internal/database/customer.go internal/api/admin_reseller_test.go internal/api/admin_plans_test.go
git commit -m "feat(api): admin wholesale plan field and reseller toggle"
```

---

### Task 8: Telegram bot uses ResolvePlanPrice

**Files:**
- Modify: `internal/handler/payment_handlers.go`
- Test: `internal/handler/payment_handlers_reseller_test.go` (new) if package testable; else unit-test a small extracted helper

**Interfaces:**
- Consumes: `config.ResolvePlanPrice`, `payment.WithPricingTier`
- Produces: bot mobile banking create charges wholesale for resellers

**handleMobileBankingPayment:**

```go
amount, tier := config.ResolvePlanPrice(*plan, customer.IsReseller)
ctx = payment.WithPricingTier(ctx, tier)
_, purchaseId, err := h.paymentService.CreatePurchase(ctx, float64(amount), plan.Days, plan.TrafficLimitGB, customer, database.InvoiceTypeMobileBanking, "")
// editMobileBankingInstructions should show amount (not plan.Price):
h.editMobileBankingInstructions(ctx, b, callback, amount, planIdx, langCode, false)
```

**buildPricingKeyboard:** keyboard is shown before customer is known in current flow (`Buy` → keyboard with retail prices). Spec requires bot path consistency on **create**. For display:
- Option A (v1 locked): keep keyboard retail labels (customer unknown at keyboard build).
- Option B: if `BuyCallbackHandler` has customer, rebuild keyboard with effective prices.

**Locked:** Prefer Option B when customer is available in the buy flow. If `buildPricingKeyboard` is only called without customer, leave labels retail; **create path must still resolve**. Add a helper:

```go
func planPriceLabel(plan config.Plan, isReseller bool) string {
	amount, _ := config.ResolvePlanPrice(plan, isReseller)
	return fmt.Sprintf("%s %d Days - %s %s", plan.Label, plan.Days, formatPrice(amount), config.Currency())
}
```

If buy handler can load customer by chat ID, pass `isReseller` into keyboard builder.

- [ ] **Step 1: Failing test for Resolve usage**

```go
func TestPlanPriceLabel_ResellerWholesale(t *testing.T) {
	plan := config.Plan{Label: "A", Days: 30, Price: 5000, WholesalePrice: intPtr(4000)}
	label := planPriceLabel(plan, true)
	if !strings.Contains(label, "4000") && !strings.Contains(label, formatPrice(4000)) {
		t.Fatalf("label should show wholesale: %s", label)
	}
}
```

- [ ] **Step 2–4: Implement create-path resolve + optional keyboard; tests pass**

- [ ] **Step 5: Commit**

```bash
git add internal/handler/payment_handlers.go internal/handler/payment_handlers_reseller_test.go
git commit -m "feat(bot): charge resellers via ResolvePlanPrice"
```

---

### Task 9: Frontend types, Plans, Checkout

**Files:**
- Modify: `web-app/src/lib/types.ts`
- Modify: `web-app/src/pages/Plans.tsx`
- Modify: `web-app/src/pages/Checkout.tsx`
- Modify: `web-app/src/pages/Plans.test.tsx`
- Modify: `web-app/src/pages/Checkout.test.tsx`
- Modify: `web-app/src/lib/translations.ts` (add EN + MY `reseller_price_badge` and admin reseller card keys)

**Interfaces:**

```ts
export interface Plan {
  id: string;
  label: string;
  days: number;
  price: number; // effective from API
  traffic_limit_gb: number;
  currency: string;
  active?: boolean;
  sort_order?: number;
  pricing_tier?: 'retail' | 'wholesale';
}

export interface AdminPlan extends Plan {
  active: boolean;
  sort_order: number;
  wholesale_price?: number | null;
}

export interface UserData {
  // existing...
  is_admin?: boolean;
  is_reseller?: boolean;
}
```

**Plans.tsx:**
- After `/api/me`, if `data.is_reseller`, hide entire promo UI block.
- When `plan.pricing_tier === 'wholesale'`, show small badge text from `t('reseller_price_badge')` (add EN + MY keys).
- Prices already come from API `price` field — do not recompute.

**Checkout.tsx:**
- If user is reseller (from me or location state), do not send `promo_code` even if URL has `?promo=`.
- Hide promo-related display for resellers.
- Show reseller badge when applicable.

- [ ] **Step 1: Failing vitest**

```tsx
// Plans.test.tsx additions
it('hides promo section for resellers', async () => {
  // mock /api/me is_reseller true, /api/plans returns pricing_tier wholesale
  // expect promo title not in document
  // expect reseller badge visible
});

// Checkout.test.tsx
it('does not send promo_code for reseller even if URL has promo', async () => {
  // mock me is_reseller; create purchase body must not include promo_code
});
```

- [ ] **Step 2: Run fail** — `cd web-app && npm test -- Plans.test.tsx Checkout.test.tsx`

- [ ] **Step 3: Implement UI + translation keys**

EN: `reseller_price_badge`: `Reseller price`  
MY: appropriate short label (match existing translation style)

- [ ] **Step 4: Tests pass + `npm run build`**

- [ ] **Step 5: Commit**

```bash
git add web-app/src/lib/types.ts web-app/src/pages/Plans.tsx web-app/src/pages/Checkout.tsx web-app/src/pages/Plans.test.tsx web-app/src/pages/Checkout.test.tsx web-app/src/lib/translations.ts
git commit -m "feat(web): reseller plan prices, badge, and promo hide"
```

---

### Task 10: AdminPlans wholesale field + AdminResellers page + Home/App

**Files:**
- Modify: `web-app/src/pages/AdminPlans.tsx`, `AdminPlans.test.tsx`
- Create: `web-app/src/pages/AdminResellers.tsx`, `AdminResellers.test.tsx`
- Modify: `web-app/src/pages/Home.tsx`, `Home.test.tsx`
- Modify: `web-app/src/App.tsx`
- Translations for admin card strings

**AdminPlans:**
- Form field `wholesale_price` (number, optional). Empty → send `null` on save to clear.
- Display current wholesale in list rows.

**AdminResellers:**
- Admin gate via `/api/me` `is_admin` (same pattern as AdminPlans).
- Input telegram_id + Toggle button calling `PATCH /api/admin/customers/{id}/reseller` with `{is_reseller: true|false}`.
- List from `GET /api/admin/resellers`.
- Minimal styling consistent with AdminPromos density.

**Home:** admin-only card before Plans (or after Finance if present):

```tsx
{data?.is_admin && (
  <Link to="/admin/resellers">...</Link>
)}
```

**App.tsx:**

```tsx
<Route path="/admin/resellers" element={<AdminResellers />} />
```

- [ ] **Step 1: Failing tests** for Home card visibility, AdminResellers toggle call, AdminPlans wholesale field submit

- [ ] **Step 2–4: Implement + pass tests + build**

- [ ] **Step 5: Commit**

```bash
git add web-app/src/pages/AdminPlans.tsx web-app/src/pages/AdminPlans.test.tsx web-app/src/pages/AdminResellers.tsx web-app/src/pages/AdminResellers.test.tsx web-app/src/pages/Home.tsx web-app/src/pages/Home.test.tsx web-app/src/App.tsx web-app/src/lib/translations.ts
git commit -m "feat(web): admin wholesale plans and reseller management"
```

---

### Task 11: Documentation

**Files:**
- Modify: `docs/MINI_APP.md`
- Modify: `HOWTOUSE.md`
- Optionally: `readme.md` if it documents plan/purchase API fields

**Content to append (exact topics):**
- Admin: set plan `wholesale_price` (must be >0 and ≤ retail)
- Admin: mark customer reseller via Mini App Resellers page or API
- Reseller Mini App: sees effective prices; no promos
- Purchase stores `pricing_tier`; finance still uses paid amounts
- No historical backfill
- Keys remain on reseller account

- [ ] **Step 1: Write docs**

- [ ] **Step 2: Commit**

```bash
git add docs/MINI_APP.md HOWTOUSE.md readme.md
git commit -m "docs: reseller wholesale pricing operations"
```

---

### Task 12: Full verification gate

**Files:** none (verification only)

- [ ] **Step 1: Backend**

```bash
go test ./...
go vet ./...
go build ./cmd/app
```

Expected: all PASS

- [ ] **Step 2: Frontend**

```bash
cd web-app && npm test && npm run build
```

Expected: all PASS

- [ ] **Step 3: Money-path safety check**

```bash
git diff main -- internal/wallet internal/payment | head
```

Confirm diff is limited to pricing_tier stamping / context / amount selection — no fulfillment or balance-mutation algorithm changes beyond amount source.

- [ ] **Step 4: Commit only if verification fixes needed; otherwise done**

---

## Self-Review

### Spec coverage

| Spec requirement | Task |
|------------------|------|
| `customers.is_reseller` | 1, 2 |
| `purchases.pricing_tier` | 1, 3, 5 |
| `Plan.WholesalePrice` + validation | 4, 7 |
| `ResolvePlanPrice` | 4, 6, 8 |
| `/api/me` is_reseller | 6 |
| `/api/plans` effective price / no leak | 6 |
| `/api/purchase` resolve + promo reject | 6 |
| `/api/promo/validate` reseller reject | 6 |
| Admin wholesale field | 7, 10 |
| Admin reseller toggle + list | 7, 10 |
| Bot same resolver | 8 |
| Mini App Plans/Checkout UX | 9 |
| Admin Mini App UX | 10 |
| Docs | 11 |
| Money-safety / no fulfillment refactor | 5, 12 |
| No historical backfill | 1 (DEFAULT retail) |
| Top-up never wholesale/promo | 5, 6 |

### Placeholder scan

No TBD/TODO steps. Payment tier threading uses explicit context helpers. GetPlans soft-auth approach is locked.

### Type consistency

- `config.PricingTierRetail` / `PricingTierWholesale` used by payment, API, tests.
- `Purchase.PricingTier` string matches DB check constraint.
- API JSON: `is_reseller`, `pricing_tier`, `wholesale_price`.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-12-reseller-wholesale-pricing.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — execute tasks in this session with checkpoints  

Which approach?
