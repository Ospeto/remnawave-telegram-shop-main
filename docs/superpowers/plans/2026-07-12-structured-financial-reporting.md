# Structured Financial Reporting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a shared, Yangon-local structured finance report (gross/refunds/net/cash/wallet/orders/customers/AOV/category/method + prior comparison + trend) with ledger-backed refunds, JSON+CSV parity, Telegram/cron reuse, and a responsive Mini App `/admin/finance` page.

**Architecture (locked):**
1. Migration `000033` creates `financial_adjustment` ledger for refund-date recognition.
2. Read-only assembly lives in **`internal/reporting/service.go`** as `FinanceService` (not `PaymentService`, not API-handler-local assembly).
3. Pure DTO math lives in `internal/reporting/finance.go` (`BuildFinanceReport`).
4. API handlers, Telegram `/revenue`, and cron all call `FinanceService.GetReport`.
5. Mini App only renders server values.
6. **Do not modify** `internal/payment` fulfillment or wallet-balance mutation code.
7. Wallet cleanup refunds (`wallet_transaction.type = 'refund'`) are **not** service refunds. Service refunds come only from `financial_adjustment` where `adjustment_type = 'refund'`.

**Tech Stack:** Go 1.25.3, Postgres 17 + SQL migrations, `internal/database` + `internal/reporting` + `internal/api`, React/Vite Mini App (`web-app/`), vitest + testing-library. **No chart library** — pure SVG/CSS. Money remains `float64` / Postgres `NUMERIC` (integer-money conversion is **out of scope**). Enforce two-decimal rounding only at report/API/CSV boundaries via `reporting.RoundMoney`.

**Source of truth:** `docs/plans/2026-07-12-financial-reporting-design.md`

**Hard constraints:**
1. Do not modify payment fulfillment / wallet mutation paths in `internal/payment`.
2. Wallet cleanup refunds ≠ service refunds.
3. No automatic historical refund backfill.
4. Integer money conversion is out of scope; keep `float64` and round to 2 decimals at boundaries.
5. JSON and CSV must derive from the same `FinanceReport` value.
6. Admin exclusions apply to every metric and export.

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `db/migrations/000033_financial_adjustment.up.sql` | Create `financial_adjustment` ledger + indexes/uniqueness |
| `db/migrations/000033_financial_adjustment.down.sql` | Drop ledger |
| `internal/database/financial_adjustment.go` | Types, repository, idempotent create, refund period sums |
| `internal/database/financial_adjustment_test.go` | Unit tests for amount/SQL/idempotency helpers |
| `internal/database/purchase.go` | Year/custom periods, inclusive range helper, distinct customer count SQL |
| `internal/database/purchase_revenue_test.go` | SQL-shape + period tests |
| `internal/reporting/money.go` | `RoundMoney` (2-decimal boundaries) |
| `internal/reporting/money_test.go` | Money rounding tests |
| `internal/reporting/ranges.go` | `ResolveReportWindow` + history bounds |
| `internal/reporting/ranges_test.go` | Window/bounds tests |
| `internal/reporting/finance.go` | `FinanceReport` DTO, `BuildFinanceReport`, prior/trend |
| `internal/reporting/finance_csv.go` | CSV encoder from `FinanceReport` |
| `internal/reporting/finance_test.go` | DTO math + CSV parity tests |
| `internal/reporting/service.go` | **Locked** read-only `FinanceService` assembly |
| `internal/reporting/service_test.go` | Service validation/error classification tests |
| `internal/reporting/revenue.go` | Zero-safe period helpers + Telegram formatters via `FinanceReport` |
| `internal/reporting/revenue_test.go` | Zero-safe + Telegram formatter tests |
| `internal/api/handlers.go` | Inject `financeService` + `financialAdjustmentRepo`; revenue/export/adjustment handlers |
| `internal/api/server.go` | Extend `RegisterHandlers` + `NewAPIHandler` wiring; register routes |
| `internal/api/revenue_test.go` | Auth/validation/bounds/export/error-class tests |
| `internal/api/financial_adjustment_test.go` | Admin create adjustment tests |
| `internal/handler/handler.go` | Add `financeService *reporting.FinanceService` field + constructor param |
| `internal/handler/admin.go` | `/revenue` uses `financeService.GetReport` |
| `cmd/app/main.go` | Construct repos + `FinanceService`; pass into API + bot handler + cron |
| `web-app/src/lib/finance.ts` | Types + format + query + SVG point helpers |
| `web-app/src/lib/finance.test.ts` | Pure helper tests |
| `web-app/src/pages/AdminFinance.tsx` | Finance page + pure SVG trend chart |
| `web-app/src/pages/AdminFinance.test.tsx` | Page state/period/export tests |
| `web-app/src/App.tsx` | Register `/admin/finance` |
| `web-app/src/pages/Home.tsx` | Admin Finance nav card |
| `web-app/src/pages/Home.test.tsx` | Finance card visibility tests |
| `web-app/src/lib/translations.ts` | Finance UI strings |
| `docs/MINI_APP.md` | Finance page + API notes |
| `HOWTOUSE.md` | Ops: refund adjustment entry, finance page, CSV |

**Explicitly not modified:** `internal/payment/payment.go` and other payment fulfillment / wallet mutation files.

---

## Shared interface contracts (Consumes / Produces)

Later tasks must use exactly these shapes.

```go
// package database

type FinancialAdjustmentType string

const FinancialAdjustmentTypeRefund FinancialAdjustmentType = "refund"

type FinancialAdjustment struct {
	ID             int64                   `json:"id"`
	PurchaseID     *int64                  `json:"purchase_id,omitempty"`
	AdjustmentType FinancialAdjustmentType `json:"adjustment_type"`
	Amount         float64                 `json:"amount"` // stored positive; reporting subtracts refunds
	Currency       string                  `json:"currency"`
	EffectiveAt    time.Time               `json:"effective_at"`
	Reason         string                  `json:"reason"`
	ExternalRef    string                  `json:"external_ref"`
	CreatedBy      string                  `json:"created_by"` // "admin:<telegram_id>" or "system"
	IdempotencyKey string                  `json:"idempotency_key"`
	CreatedAt      time.Time               `json:"created_at"`
}

type CreateFinancialAdjustmentInput struct {
	PurchaseID     *int64
	AdjustmentType FinancialAdjustmentType
	Amount         float64 // must be > 0; normalized to 2 decimals
	Currency       string
	EffectiveAt    time.Time
	Reason         string
	ExternalRef    string
	CreatedBy      string
	IdempotencyKey string // required, unique
}

type RefundPeriodRow struct {
	PeriodStart string
	Currency    string
	RefundTotal float64 // positive magnitude
	RefundCount int
}

type FinancialAdjustmentRepository struct {
	pool *pgxpool.Pool
}

func NewFinancialAdjustmentRepository(pool *pgxpool.Pool) *FinancialAdjustmentRepository

// Create is idempotent on idempotency_key: second call returns existing row, created=false.
func (r *FinancialAdjustmentRepository) Create(ctx context.Context, in CreateFinancialAdjustmentInput) (*FinancialAdjustment, bool, error)

func (r *FinancialAdjustmentRepository) SumRefundsByPeriod(ctx context.Context, start, end time.Time, period RevenueSummaryPeriod, adminTelegramID int64) ([]RefundPeriodRow, error)

const (
	RevenuePeriodDay    RevenueSummaryPeriod = "day"
	RevenuePeriodWeek   RevenueSummaryPeriod = "week"
	RevenuePeriodMonth  RevenueSummaryPeriod = "month"
	RevenuePeriodYear   RevenueSummaryPeriod = "year"
	RevenuePeriodCustom RevenueSummaryPeriod = "custom"
)

func NormalizeRevenueSummaryPeriod(period string) (RevenueSummaryPeriod, error)
func InclusiveYangonDateRangeToHalfOpen(from, to time.Time) (start, end time.Time, err error)
func (pr *PurchaseRepository) GetRevenueSummaryRange(ctx context.Context, start, end time.Time, period RevenueSummaryPeriod) ([]RevenueSummaryRow, error)
func (pr *PurchaseRepository) CountDistinctServiceCustomers(ctx context.Context, start, end time.Time) (int, error)
```

```go
// package reporting

func RoundMoney(v float64) float64 // half-away-from-zero to 2 decimals

type MoneyDelta struct {
	Absolute   float64  `json:"absolute"`
	Percentage *float64 `json:"percentage"` // null when prior base is 0
}

type FinanceMetrics struct {
	GrossServiceRevenue float64 `json:"gross_service_revenue"`
	Refunds             float64 `json:"refunds"`               // positive magnitude
	NetServiceRevenue   float64 `json:"net_service_revenue"`   // gross - refunds; UI "Net Income"
	CashCollected       float64 `json:"cash_collected"`
	WalletTopUps        float64 `json:"wallet_topups"`
	WalletSpend         float64 `json:"wallet_spend"`
	SuccessfulOrders    int     `json:"successful_orders"` // plan purchases only
	UniqueCustomers     int     `json:"unique_customers"`  // distinct across full range
	AverageOrderValue   float64 `json:"average_order_value"` // 0 when orders == 0
	NewSubscriptions    int     `json:"new_subscriptions"`
	Extensions          int     `json:"extensions"`
}

type FinanceDelta struct {
	GrossServiceRevenue MoneyDelta `json:"gross_service_revenue"`
	Refunds             MoneyDelta `json:"refunds"`
	NetServiceRevenue   MoneyDelta `json:"net_service_revenue"`
	CashCollected       MoneyDelta `json:"cash_collected"`
}

type CategoryBreakdown struct {
	Category string  `json:"category"` // new_key | extension | wallet_topup
	Orders   int     `json:"orders"`
	Amount   float64 `json:"amount"`
}

type MethodBreakdown struct {
	Method         string  `json:"method"`
	Transactions   int     `json:"transactions"`
	ServiceRevenue float64 `json:"service_revenue"`
	CashCollected  float64 `json:"cash_collected"`
	WalletTopUps   float64 `json:"wallet_topups"`
	WalletSpend    float64 `json:"wallet_spend"`
}

type FinanceTrendBucket struct {
	PeriodStart string              `json:"period_start"` // YYYY-MM-DD Yangon
	PeriodEnd   string              `json:"period_end"`   // inclusive local date
	InProgress  bool                `json:"in_progress"`
	Metrics     FinanceMetrics      `json:"metrics"`
	Categories  []CategoryBreakdown `json:"categories"` // period-specific only
	Methods     []MethodBreakdown   `json:"methods"`    // period-specific only
}

type FinanceReport struct {
	Period      string          `json:"period"` // day|week|month|year|custom
	Timezone    string          `json:"timezone"` // Asia/Yangon
	Currency    string          `json:"currency"`
	RangeStart  string          `json:"range_start"` // inclusive YYYY-MM-DD
	RangeEnd    string          `json:"range_end"`   // inclusive YYYY-MM-DD
	GeneratedAt time.Time       `json:"generated_at"`
	InProgress  bool            `json:"in_progress"`
	Current     FinanceMetrics  `json:"current"`
	Prior       *FinanceMetrics `json:"prior"`
	Delta       *FinanceDelta   `json:"delta"`
	// Categories/Methods are range-level totals for the selected window (summary cards / CSV).
	Categories []CategoryBreakdown  `json:"categories"`
	Methods    []MethodBreakdown    `json:"methods"`
	// Trend buckets each carry their own Categories/Methods for expandable historical rows.
	Trend []FinanceTrendBucket `json:"trend"` // ascending
}

type BuildFinanceReportInput struct {
	Period               database.RevenueSummaryPeriod
	Now                  time.Time
	HistoryPeriods       int
	CustomFrom           *time.Time
	CustomTo             *time.Time
	PurchaseRows         []database.RevenueSummaryRow
	RefundRows           []database.RefundPeriodRow
	PriorPurchaseRows    []database.RevenueSummaryRow
	PriorRefundRows      []database.RefundPeriodRow
	RangeUniqueCustomers int
	PriorUniqueCustomers int
	// Window fields filled by FinanceService before BuildFinanceReport:
	CurrentStart time.Time
	CurrentEnd   time.Time // half-open
	PriorStart   time.Time
	PriorEnd     time.Time // half-open
}

func BuildFinanceReport(in BuildFinanceReportInput) (FinanceReport, error)
func FormatFinanceReportCSV(report FinanceReport) ([]byte, error)
func FormatTelegramFinanceReport(title string, report FinanceReport) string
func FormatRevenueCommandFromReport(report FinanceReport, today string) string

type ReportQuery struct {
	Period         database.RevenueSummaryPeriod
	HistoryPeriods int        // 0 => default for period
	CustomFrom     *time.Time // inclusive Yangon date-only; required for custom
	CustomTo       *time.Time // inclusive Yangon date-only; required for custom
	Now            time.Time  // zero => time.Now()
}

// ErrInvalidReportQuery is returned for validation failures (API maps to 400).
var ErrInvalidReportQuery = errors.New("invalid report query")

type purchaseRevenueReader interface {
	GetRevenueSummaryRange(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod) ([]database.RevenueSummaryRow, error)
	CountDistinctServiceCustomers(ctx context.Context, start, end time.Time) (int, error)
}

type refundPeriodReader interface {
	SumRefundsByPeriod(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod, adminTelegramID int64) ([]database.RefundPeriodRow, error)
}

type FinanceService struct {
	purchases purchaseRevenueReader
	refunds   refundPeriodReader
	adminID   func() int64 // returns config.GetAdminTelegramId
	now       func() time.Time
}

func NewFinanceService(purchases purchaseRevenueReader, refunds refundPeriodReader) *FinanceService

func (s *FinanceService) GetReport(ctx context.Context, q ReportQuery) (FinanceReport, error)

func ResolveReportWindow(period database.RevenueSummaryPeriod, now time.Time, historyPeriods int, customFrom, customTo *time.Time) (currentStart, currentEnd, priorStart, priorEnd time.Time, err error)
```

```go
// package api — exact injection (no constructor ambiguity)

// Existing clock support on APIHandler (already in handlers.go; keep and use):
//   now func() time.Time
//   NewAPIHandler sets now: time.Now
//   currentTime() returns h.now() when non-nil, else time.Now()
// Tests may override: handler.now = func() time.Time { return fixed }
// All finance handlers MUST use h.currentTime() (not bare time.Now) for Now/effective_at defaults.

// APIHandler gains two fields (set only via NewAPIHandler):
//   financeService            financeReporter
//   financialAdjustmentRepo   financialAdjustmentCreator
//
// Interfaces defined in handlers.go:
type financeReporter interface {
	GetReport(ctx context.Context, q reporting.ReportQuery) (reporting.FinanceReport, error)
}

type financialAdjustmentCreator interface {
	Create(ctx context.Context, in database.CreateFinancialAdjustmentInput) (*database.FinancialAdjustment, bool, error)
}

// NewAPIHandler signature becomes (append two params at end):
func NewAPIHandler(
	customerRepo *database.CustomerRepository,
	paymentService *payment.PaymentService,
	telegramBot *bot.Bot,
	tm *translation.Manager,
	subKeyRepo *database.SubscriptionKeyRepository,
	promoCodeRepository *database.PromoCodeRepository,
	walletService *walletsvc.WalletService,
	referralRepo *database.ReferralRepository,
	appConfigRepo *database.AppConfigRepository,
	financeService financeReporter,
	financialAdjustmentRepo financialAdjustmentCreator,
) *APIHandler

// NewAPIHandler return literal must include (existing fields unchanged) plus:
//   screenshotAttempts: make(map[int64]time.Time),
//   customerScreenshotAttempts: make(map[int64][]time.Time),
//   screenshotInFlight: make(map[int64]struct{}),
//   now: time.Now, // REQUIRED clock; tests may override h.now
//   financeService: financeService,
//   financialAdjustmentRepo: financialAdjustmentRepo,
//
// currentTime remains:
//   func (h *APIHandler) currentTime() time.Time {
//     if h != nil && h.now != nil { return h.now() }
//     return time.Now()
//   }
// CreateFinancialAdjustment and parseReportQuery use h.currentTime() only.

// RegisterHandlers signature becomes (append two params at end):
func RegisterHandlers(
	mux *http.ServeMux,
	customerRepo *database.CustomerRepository,
	paymentService *payment.PaymentService,
	telegramBot *bot.Bot,
	tm *translation.Manager,
	subKeyRepo *database.SubscriptionKeyRepository,
	promoCodeRepository *database.PromoCodeRepository,
	walletService *walletsvc.WalletService,
	referralRepo *database.ReferralRepository,
	appConfigRepo *database.AppConfigRepository,
	financeService *reporting.FinanceService,
	financialAdjustmentRepo *database.FinancialAdjustmentRepository,
)

// Authenticated admin telegram id (same pattern as existing handlers):
//   telegramID, ok := r.Context().Value(telegramIDKey).(int64)
// telegramIDKey is package-private in server.go (contextKey = "telegram_id").
// Routes use withAdmin(...), so non-admin never reaches the handler.
// created_by = fmt.Sprintf("admin:%d", telegramID)

// Error mapping rule for finance reads:
//   errors.Is(err, reporting.ErrInvalidReportQuery) => 400 with err.Error()
//   any other error from FinanceService/repos => writeSanitizedError(..., 500, ...)
```

```ts
// web-app/src/lib/finance.ts
export type FinancePeriod = 'day' | 'week' | 'month' | 'year' | 'custom';

export interface MoneyDelta {
  absolute: number;
  percentage: number | null;
}

export interface FinanceMetrics {
  gross_service_revenue: number;
  refunds: number;
  net_service_revenue: number;
  cash_collected: number;
  wallet_topups: number;
  wallet_spend: number;
  successful_orders: number;
  unique_customers: number;
  average_order_value: number;
  new_subscriptions: number;
  extensions: number;
}

export interface FinanceDelta {
  gross_service_revenue: MoneyDelta;
  refunds: MoneyDelta;
  net_service_revenue: MoneyDelta;
  cash_collected: MoneyDelta;
}

export interface CategoryBreakdown {
  category: string;
  orders: number;
  amount: number;
}

export interface MethodBreakdown {
  method: string;
  transactions: number;
  service_revenue: number;
  cash_collected: number;
  wallet_topups: number;
  wallet_spend: number;
}

export interface FinanceTrendBucket {
  period_start: string;
  period_end: string;
  in_progress: boolean;
  metrics: FinanceMetrics;
  categories: CategoryBreakdown[];
  methods: MethodBreakdown[];
}

export interface FinanceReport {
  period: FinancePeriod;
  timezone: string;
  currency: string;
  range_start: string;
  range_end: string;
  generated_at: string;
  in_progress: boolean;
  current: FinanceMetrics;
  prior: FinanceMetrics | null;
  delta: FinanceDelta | null;
  categories: CategoryBreakdown[];
  methods: MethodBreakdown[];
  trend: FinanceTrendBucket[];
}

export function formatMoneyMMK(n: number): string;
export function formatDelta(d: MoneyDelta): string;
export function buildRevenueQuery(opts: {
  period: FinancePeriod;
  periods?: number;
  from?: string;
  to?: string;
}): string;
export function buildRevenueExportQuery(opts: {
  period: FinancePeriod;
  periods?: number;
  from?: string;
  to?: string;
}): string;
export function buildTrendPolylinePoints(
  values: number[],
  width: number,
  height: number,
  pad: number,
): string;
```

---

### Task 1: Financial adjustment ledger migration (000033)

**Files:**
- Create: `db/migrations/000033_financial_adjustment.up.sql`
- Create: `db/migrations/000033_financial_adjustment.down.sql`

**Produces:** durable `financial_adjustment` table with idempotency uniqueness.

- [ ] **Step 1: Write the up migration**

```sql
-- db/migrations/000033_financial_adjustment.up.sql
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
```

- [ ] **Step 2: Write the down migration**

```sql
-- db/migrations/000033_financial_adjustment.down.sql
DROP INDEX IF EXISTS financial_adjustment_type_effective_idx;
DROP INDEX IF EXISTS financial_adjustment_purchase_id_idx;
DROP INDEX IF EXISTS financial_adjustment_effective_at_idx;
DROP INDEX IF EXISTS financial_adjustment_idempotency_key_uidx;
DROP TABLE IF EXISTS financial_adjustment;
```

- [ ] **Step 3: Sanity-check SQL files exist and are paired**

Run: `ls db/migrations/000033_financial_adjustment.*`
Expected: both `.up.sql` and `.down.sql` listed.

- [ ] **Step 4: Commit**

```bash
git add db/migrations/000033_financial_adjustment.up.sql db/migrations/000033_financial_adjustment.down.sql
git commit -m "feat(db): add financial_adjustment ledger migration 000033"
```

---

### Task 2: FinancialAdjustment repository + idempotent create

**Files:**
- Create: `internal/database/financial_adjustment.go`
- Create: `internal/database/financial_adjustment_test.go`

**Consumes:** Task 1 table shape.  
**Produces:** `FinancialAdjustmentRepository` with `Create` and `SumRefundsByPeriod` from the contract.

- [ ] **Step 1: Write failing unit tests**

```go
// internal/database/financial_adjustment_test.go
package database

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeAdjustmentAmount_TwoDecimals(t *testing.T) {
	got, err := normalizeAdjustmentAmount(10.005)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != 10.01 {
		t.Fatalf("got %v want 10.01", got)
	}
}

func TestNormalizeAdjustmentAmount_RejectsNonPositive(t *testing.T) {
	if _, err := normalizeAdjustmentAmount(0); err == nil {
		t.Fatal("expected error for zero amount")
	}
	if _, err := normalizeAdjustmentAmount(-1); err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestBuildCreateFinancialAdjustmentSQL_UsesIdempotencyConflict(t *testing.T) {
	sql := buildCreateFinancialAdjustmentSQL()
	if !strings.Contains(sql, "INSERT INTO financial_adjustment") {
		t.Fatalf("missing insert: %s", sql)
	}
	if !strings.Contains(sql, "ON CONFLICT (idempotency_key) DO NOTHING") {
		t.Fatalf("missing idempotency conflict clause: %s", sql)
	}
}

func TestBuildSumRefundsByPeriodSQL_YangonAndAdminExclusion(t *testing.T) {
	sql, err := buildSumRefundsByPeriodSQL(RevenuePeriodDay)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Asia/Yangon",
		"adjustment_type = 'refund'",
		"telegram_id",
		"effective_at >= $1",
		"effective_at < $2",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("missing %q in %s", want, sql)
		}
	}
}

func TestCreateFinancialAdjustmentInput_RequiresIdempotencyKey(t *testing.T) {
	err := validateCreateFinancialAdjustmentInput(CreateFinancialAdjustmentInput{
		AdjustmentType: FinancialAdjustmentTypeRefund,
		Amount:         100,
		Currency:       "MMK",
		EffectiveAt:    time.Now(),
		CreatedBy:      "admin:1",
		IdempotencyKey: "",
	})
	if err == nil {
		t.Fatal("expected missing idempotency key error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/database/ -run 'TestNormalizeAdjustmentAmount|TestBuildCreateFinancialAdjustmentSQL|TestBuildSumRefundsByPeriodSQL|TestCreateFinancialAdjustmentInput' -count=1`
Expected: FAIL with undefined symbols (`normalizeAdjustmentAmount`, `buildCreateFinancialAdjustmentSQL`, etc.).

- [ ] **Step 3: Implement repository + helpers**

```go
// internal/database/financial_adjustment.go
package database

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"remnawave-tg-shop-bot/internal/config"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type FinancialAdjustmentType string

const FinancialAdjustmentTypeRefund FinancialAdjustmentType = "refund"

type FinancialAdjustment struct {
	ID             int64                   `json:"id"`
	PurchaseID     *int64                  `json:"purchase_id,omitempty"`
	AdjustmentType FinancialAdjustmentType `json:"adjustment_type"`
	Amount         float64                 `json:"amount"`
	Currency       string                  `json:"currency"`
	EffectiveAt    time.Time               `json:"effective_at"`
	Reason         string                  `json:"reason"`
	ExternalRef    string                  `json:"external_ref"`
	CreatedBy      string                  `json:"created_by"`
	IdempotencyKey string                  `json:"idempotency_key"`
	CreatedAt      time.Time               `json:"created_at"`
}

type CreateFinancialAdjustmentInput struct {
	PurchaseID     *int64
	AdjustmentType FinancialAdjustmentType
	Amount         float64
	Currency       string
	EffectiveAt    time.Time
	Reason         string
	ExternalRef    string
	CreatedBy      string
	IdempotencyKey string
}

type RefundPeriodRow struct {
	PeriodStart string
	Currency    string
	RefundTotal float64
	RefundCount int
}

type FinancialAdjustmentRepository struct {
	pool *pgxpool.Pool
}

func NewFinancialAdjustmentRepository(pool *pgxpool.Pool) *FinancialAdjustmentRepository {
	return &FinancialAdjustmentRepository{pool: pool}
}

func normalizeAdjustmentAmount(amount float64) (float64, error) {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0, fmt.Errorf("amount must be positive")
	}
	scaled := amount * 100
	if scaled >= 0 {
		scaled = math.Floor(scaled + 0.5)
	} else {
		scaled = math.Ceil(scaled - 0.5)
	}
	out := scaled / 100
	if out <= 0 {
		return 0, fmt.Errorf("amount must be positive after rounding")
	}
	return out, nil
}

func validateCreateFinancialAdjustmentInput(in CreateFinancialAdjustmentInput) error {
	if in.AdjustmentType != FinancialAdjustmentTypeRefund {
		return fmt.Errorf("unsupported adjustment_type: %s", in.AdjustmentType)
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		return fmt.Errorf("idempotency_key is required")
	}
	if strings.TrimSpace(in.CreatedBy) == "" {
		return fmt.Errorf("created_by is required")
	}
	if _, err := normalizeAdjustmentAmount(in.Amount); err != nil {
		return err
	}
	return nil
}

func buildCreateFinancialAdjustmentSQL() string {
	return `
		INSERT INTO financial_adjustment (
			purchase_id, adjustment_type, amount, currency, effective_at,
			reason, external_ref, created_by, idempotency_key
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, purchase_id, adjustment_type, amount, currency, effective_at,
		          reason, external_ref, created_by, idempotency_key, created_at`
}

func buildSumRefundsByPeriodSQL(period RevenueSummaryPeriod) (string, error) {
	var bucket string
	switch period {
	case RevenuePeriodDay, RevenuePeriodCustom:
		bucket = `(fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodWeek:
		bucket = `DATE_TRUNC('week', fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodMonth:
		bucket = `DATE_TRUNC('month', fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	case RevenuePeriodYear:
		bucket = `DATE_TRUNC('year', fa.effective_at AT TIME ZONE 'Asia/Yangon')::date`
	default:
		return "", fmt.Errorf("unsupported revenue period: %s", period)
	}
	return fmt.Sprintf(`
		SELECT
			%s AS period_start,
			COALESCE(NULLIF(fa.currency, ''), 'MMK') AS currency,
			COALESCE(SUM(fa.amount), 0) AS refund_total,
			COUNT(*) AS refund_count
		FROM financial_adjustment fa
		LEFT JOIN purchase p ON p.id = fa.purchase_id
		LEFT JOIN customer c ON c.id = p.customer_id
		WHERE fa.adjustment_type = 'refund'
		  AND fa.effective_at >= $1
		  AND fa.effective_at < $2
		  AND ($3::bigint = 0 OR c.telegram_id IS NULL OR c.telegram_id <> $3)
		GROUP BY 1, 2
		ORDER BY 1 ASC`, bucket), nil
}

func (r *FinancialAdjustmentRepository) Create(ctx context.Context, in CreateFinancialAdjustmentInput) (*FinancialAdjustment, bool, error) {
	if err := validateCreateFinancialAdjustmentInput(in); err != nil {
		return nil, false, err
	}
	amount, err := normalizeAdjustmentAmount(in.Amount)
	if err != nil {
		return nil, false, err
	}
	currency := strings.TrimSpace(in.Currency)
	if currency == "" {
		currency = "MMK"
	}

	row := &FinancialAdjustment{}
	err = r.pool.QueryRow(ctx, buildCreateFinancialAdjustmentSQL(),
		in.PurchaseID, string(in.AdjustmentType), amount, currency, in.EffectiveAt,
		in.Reason, in.ExternalRef, in.CreatedBy, strings.TrimSpace(in.IdempotencyKey),
	).Scan(
		&row.ID, &row.PurchaseID, &row.AdjustmentType, &row.Amount, &row.Currency, &row.EffectiveAt,
		&row.Reason, &row.ExternalRef, &row.CreatedBy, &row.IdempotencyKey, &row.CreatedAt,
	)
	if err == nil {
		return row, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("insert financial_adjustment: %w", err)
	}

	existing := &FinancialAdjustment{}
	err = r.pool.QueryRow(ctx, `
		SELECT id, purchase_id, adjustment_type, amount, currency, effective_at,
		       reason, external_ref, created_by, idempotency_key, created_at
		FROM financial_adjustment WHERE idempotency_key = $1`, strings.TrimSpace(in.IdempotencyKey),
	).Scan(
		&existing.ID, &existing.PurchaseID, &existing.AdjustmentType, &existing.Amount, &existing.Currency, &existing.EffectiveAt,
		&existing.Reason, &existing.ExternalRef, &existing.CreatedBy, &existing.IdempotencyKey, &existing.CreatedAt,
	)
	if err != nil {
		return nil, false, fmt.Errorf("load existing financial_adjustment: %w", err)
	}
	return existing, false, nil
}

func (r *FinancialAdjustmentRepository) SumRefundsByPeriod(ctx context.Context, start, end time.Time, period RevenueSummaryPeriod, adminTelegramID int64) ([]RefundPeriodRow, error) {
	query, err := buildSumRefundsByPeriodSQL(period)
	if err != nil {
		return nil, err
	}
	if adminTelegramID == 0 {
		adminTelegramID = config.GetAdminTelegramId()
	}
	rows, err := r.pool.Query(ctx, query, start, end, adminTelegramID)
	if err != nil {
		return nil, fmt.Errorf("sum refunds by period: %w", err)
	}
	defer rows.Close()
	var out []RefundPeriodRow
	for rows.Next() {
		var rr RefundPeriodRow
		var periodStart time.Time
		if err := rows.Scan(&periodStart, &rr.Currency, &rr.RefundTotal, &rr.RefundCount); err != nil {
			return nil, err
		}
		rr.PeriodStart = periodStart.Format("2006-01-02")
		out = append(out, rr)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/database/ -run 'TestNormalizeAdjustmentAmount|TestBuildCreateFinancialAdjustmentSQL|TestBuildSumRefundsByPeriodSQL|TestCreateFinancialAdjustmentInput' -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/financial_adjustment.go internal/database/financial_adjustment_test.go
git commit -m "feat(db): financial adjustment repository with idempotent create"
```

---

### Task 3: Year + custom period support and Yangon range helpers

**Files:**
- Modify: `internal/database/purchase.go`
- Modify: `internal/database/purchase_revenue_test.go`
- Modify: `internal/reporting/revenue.go`

**Consumes:** existing day/week/month helpers.  
**Produces:** `RevenuePeriodYear`, `RevenuePeriodCustom`, year bucketing, `InclusiveYangonDateRangeToHalfOpen`, `StartOfYear`, `PreviousYearRange`.

- [ ] **Step 1: Write failing period tests**

```go
// append to internal/database/purchase_revenue_test.go
func TestNormalizeRevenueSummaryPeriod_YearAndCustom(t *testing.T) {
	y, err := NormalizeRevenueSummaryPeriod("year")
	if err != nil || y != RevenuePeriodYear {
		t.Fatalf("year: got %q err=%v", y, err)
	}
	c, err := NormalizeRevenueSummaryPeriod("custom")
	if err != nil || c != RevenuePeriodCustom {
		t.Fatalf("custom: got %q err=%v", c, err)
	}
	if _, err := NormalizeRevenueSummaryPeriod("quarter"); err == nil {
		t.Fatal("expected error for quarter")
	}
}

func TestBuildRevenueSummaryQuery_YearBucket(t *testing.T) {
	q, err := buildRevenueSummaryQuery(RevenuePeriodYear)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "DATE_TRUNC('year'") {
		t.Fatalf("missing year trunc: %s", q)
	}
	if !strings.Contains(q, "Asia/Yangon") {
		t.Fatalf("missing Yangon: %s", q)
	}
}

func TestInclusiveYangonDateRangeToHalfOpen(t *testing.T) {
	loc := revenueSummaryLocation()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	start, end, err := InclusiveYangonDateRangeToHalfOpen(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if !start.Equal(from) {
		t.Fatalf("start=%v want %v", start, from)
	}
	wantEnd := time.Date(2026, 2, 1, 0, 0, 0, 0, loc)
	if !end.Equal(wantEnd) {
		t.Fatalf("end=%v want %v", end, wantEnd)
	}
}
```

- [ ] **Step 2: Run tests to verify fail**

Run: `go test ./internal/database/ -run 'TestNormalizeRevenueSummaryPeriod_YearAndCustom|TestBuildRevenueSummaryQuery_YearBucket|TestInclusiveYangonDateRangeToHalfOpen' -count=1`
Expected: FAIL (unsupported period / undefined helper).

- [ ] **Step 3: Implement period extensions**

In `internal/database/purchase.go` replace period constants and helpers:

```go
const (
	RevenuePeriodDay    RevenueSummaryPeriod = "day"
	RevenuePeriodWeek   RevenueSummaryPeriod = "week"
	RevenuePeriodMonth  RevenueSummaryPeriod = "month"
	RevenuePeriodYear   RevenueSummaryPeriod = "year"
	RevenuePeriodCustom RevenueSummaryPeriod = "custom"
)

func NormalizeRevenueSummaryPeriod(period string) (RevenueSummaryPeriod, error) {
	switch RevenueSummaryPeriod(period) {
	case "", RevenuePeriodDay:
		return RevenuePeriodDay, nil
	case RevenuePeriodWeek:
		return RevenuePeriodWeek, nil
	case RevenuePeriodMonth:
		return RevenuePeriodMonth, nil
	case RevenuePeriodYear:
		return RevenuePeriodYear, nil
	case RevenuePeriodCustom:
		return RevenuePeriodCustom, nil
	default:
		return "", fmt.Errorf("unsupported revenue period: %s", period)
	}
}

func revenuePeriodExpression(period RevenueSummaryPeriod) (string, error) {
	switch period {
	case RevenuePeriodDay, RevenuePeriodCustom:
		return fmt.Sprintf("(p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	case RevenuePeriodWeek:
		return fmt.Sprintf("DATE_TRUNC('week', p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	case RevenuePeriodMonth:
		return fmt.Sprintf("DATE_TRUNC('month', p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	case RevenuePeriodYear:
		return fmt.Sprintf("DATE_TRUNC('year', p.paid_at AT TIME ZONE '%s')::date", revenueSummaryTimezone), nil
	default:
		return "", fmt.Errorf("unsupported revenue period: %s", period)
	}
}

func InclusiveYangonDateRangeToHalfOpen(from, to time.Time) (time.Time, time.Time, error) {
	loc := revenueSummaryLocation()
	start := time.Date(from.In(loc).Year(), from.In(loc).Month(), from.In(loc).Day(), 0, 0, 0, 0, loc)
	endDay := time.Date(to.In(loc).Year(), to.In(loc).Month(), to.In(loc).Day(), 0, 0, 0, 0, loc)
	if endDay.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("to must be on or after from")
	}
	return start, endDay.AddDate(0, 0, 1), nil
}

func startOfRevenueYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}
```

Extend `GetRevenueSummaryForPeriods` switch with:

```go
case RevenuePeriodYear:
	end = startOfRevenueYear(now).AddDate(1, 0, 0)
	start = end.AddDate(-periods, 0, 0)
```

In `internal/reporting/revenue.go` add:

```go
func StartOfYear(t time.Time) time.Time {
	return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
}

func PreviousYearRange(now time.Time) (time.Time, time.Time) {
	end := StartOfYear(now)
	return end.AddDate(-1, 0, 0), end
}
```

- [ ] **Step 4: Run tests to verify pass**

Run: `go test ./internal/database/ -run 'TestNormalizeRevenueSummaryPeriod|TestBuildRevenueSummaryQuery|TestInclusiveYangonDateRangeToHalfOpen' -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/database/purchase.go internal/database/purchase_revenue_test.go internal/reporting/revenue.go
git commit -m "feat(reporting): support year and custom Yangon revenue periods"
```

---

### Task 4: Money boundary helpers + zero-safe period totals

**Files:**
- Create: `internal/reporting/money.go`
- Create: `internal/reporting/money_test.go`
- Modify: `internal/reporting/revenue.go`
- Modify: `internal/reporting/revenue_test.go`

**Consumes:** existing `SummarizeRevenuePeriod`.  
**Produces:** `RoundMoney`; zero-safe period aggregation rule below.

**Exact compatibility rule (replaces firstPositive presence myth):**

`buildRevenueSummaryQuery` always SELECTs both breakdown columns (`service_revenue`, …) and period aggregate columns (`period_service_revenue`, …). After `rows.Scan`, every `Period*` field on `RevenueSummaryRow` is populated for every returned row, including legitimate zeros. Therefore:

1. When aggregating **period-level totals**, use `Period*` fields only. Deduplicate by `(PeriodStart, Currency)` so multi-breakdown rows in the same bucket do not double-count period aggregates.
2. Do **not** use `firstPositive` / `firstPositiveFloat` for period fields. Those helpers treat `0` as missing and incorrectly fall through to breakdown/`TotalRevenue` values.
3. When aggregating **method/category breakdowns**, sum breakdown fields (`ServiceRevenue`, `CashCollected`, …) across rows. Use `ServiceRevenue` directly; do not fall back from zero `ServiceRevenue` to `TotalRevenue`.
4. Range-level unique customers are **never** the sum of `PeriodUniqueCustomers`. They come from `CountDistinctServiceCustomers` supplied as `RangeUniqueCustomers`.

- [ ] **Step 1: Write failing money + zero-safe tests**

```go
// internal/reporting/money_test.go
package reporting

import "testing"

func TestRoundMoney_TwoDecimals(t *testing.T) {
	if RoundMoney(1.005) != 1.01 {
		t.Fatalf("1.005 -> %v", RoundMoney(1.005))
	}
	if RoundMoney(2.004) != 2.00 {
		t.Fatalf("2.004 -> %v", RoundMoney(2.004))
	}
	if RoundMoney(-1.005) != -1.01 {
		t.Fatalf("-1.005 -> %v", RoundMoney(-1.005))
	}
}
```

```go
// append to internal/reporting/revenue_test.go
func TestSummarizeRevenuePeriod_PreservesZeroServiceRevenue(t *testing.T) {
	rows := []database.RevenueSummaryRow{{
		PeriodStart:          "2026-07-01",
		Currency:             "MMK",
		PeriodServiceRevenue: 0,
		PeriodCashCollected:  0,
		PeriodTotalPurchases: 0,
		ServiceRevenue:       0,
		TotalRevenue:         0,
	}}
	totals, _ := SummarizeRevenuePeriod(rows)
	if totals.ServiceRevenue != 0 {
		t.Fatalf("service=%v want 0", totals.ServiceRevenue)
	}
}

func TestSummarizeRevenuePeriod_PeriodZeroWinsOverBreakdownFallback(t *testing.T) {
	rows := []database.RevenueSummaryRow{{
		PeriodStart:          "2026-07-01",
		Currency:             "MMK",
		PeriodServiceRevenue: 0,
		TotalRevenue:         999,
		ServiceRevenue:       999,
	}}
	totals, _ := SummarizeRevenuePeriod(rows)
	if totals.ServiceRevenue != 0 {
		t.Fatalf("got %v want 0 (period field is authoritative including zero)", totals.ServiceRevenue)
	}
}
```

- [ ] **Step 2: Run tests expecting fail**

Run: `go test ./internal/reporting/ -run 'TestRoundMoney|TestSummarizeRevenuePeriod_PreservesZero|TestSummarizeRevenuePeriod_PeriodZeroWins' -count=1`
Expected: FAIL (`RoundMoney` undefined and/or zero collapsed via `firstPositiveFloat`).

- [ ] **Step 3: Implement money helpers and fix SummarizeRevenuePeriod**

```go
// internal/reporting/money.go
package reporting

import "math"

func RoundMoney(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	scaled := v * 100
	if scaled >= 0 {
		scaled = math.Floor(scaled + 0.5)
	} else {
		scaled = math.Ceil(scaled - 0.5)
	}
	return scaled / 100
}
```

In `SummarizeRevenuePeriod`, replace period-field selection that uses `firstPositive` / `firstPositiveFloat` with direct `Period*` assignment on first sight of each `(PeriodStart, Currency)` key:

```go
if _, ok := periods[key]; !ok {
	periods[key] = RevenuePeriodTotals{
		PeriodStart:          start,
		Currency:             currency,
		TotalPurchases:       row.PeriodTotalPurchases,
		ServicePurchases:     row.PeriodServicePurchases,
		UniqueCustomers:      row.PeriodUniqueCustomers, // per-bucket only; not range-level
		CashCollected:        row.PeriodCashCollected,
		WalletTopUps:         row.PeriodWalletTopUps,
		WalletSpend:          row.PeriodWalletSpend,
		ServiceRevenue:       row.PeriodServiceRevenue,
		NewKeyPurchases:      row.PeriodNewKeyPurchases,
		ExtensionPurchases:   row.PeriodExtensionPurchases,
		WalletTopUpPurchases: row.PeriodWalletTopUpPurchases,
	}
}
```

For method aggregation use breakdown fields only:

```go
total.ServiceRevenue += row.ServiceRevenue
total.CashCollected += row.CashCollected
total.WalletTopUps += row.WalletTopUps
total.WalletSpend += row.WalletSpend
total.Transactions += row.TotalPurchases
```

Delete `firstPositive` and `firstPositiveFloat` from `revenue.go` if nothing else references them after this change. Do not use them for period totals. Update any tests that depended on positive-only coalesce.

- [ ] **Step 4: Run tests expecting pass**

Run: `go test ./internal/reporting/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/reporting/money.go internal/reporting/money_test.go internal/reporting/revenue.go internal/reporting/revenue_test.go
git commit -m "fix(reporting): two-decimal money boundaries and zero-safe period totals"
```

---

### Task 5: FinanceReport builder (metrics, prior comparison, trend)

**Files:**
- Create: `internal/reporting/finance.go`
- Create: `internal/reporting/finance_test.go`

**Consumes:** `BuildFinanceReportInput`, `RoundMoney`, `FinanceDelta`.  
**Produces:** `BuildFinanceReport` returning full `FinanceReport`.

- [ ] **Step 1: Write failing builder tests**

```go
package reporting

import (
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func emptyMetrics() FinanceMetrics { return FinanceMetrics{} }

func TestBuildFinanceReport_NetEqualsGrossMinusRefunds(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, loc)
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          now,
		CurrentStart: time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart:              "2026-07-12",
			Currency:                 "MMK",
			PeriodServiceRevenue:     1000,
			PeriodCashCollected:      800,
			PeriodWalletTopUps:       200,
			PeriodWalletSpend:        200,
			PeriodServicePurchases:   2,
			PeriodNewKeyPurchases:    1,
			PeriodExtensionPurchases: 1,
			RevenueCategory:          "new_key",
			PaymentMethod:            "kbz",
			ServiceRevenue:           600,
			CashCollected:            600,
			TotalPurchases:           1,
		}, {
			PeriodStart:              "2026-07-12",
			Currency:                 "MMK",
			RevenueCategory:          "extension",
			PaymentMethod:            "wallet",
			ServiceRevenue:           400,
			WalletSpend:              400,
			TotalPurchases:           1,
			PeriodServiceRevenue:     1000,
			PeriodCashCollected:      800,
			PeriodWalletTopUps:       200,
			PeriodWalletSpend:        200,
			PeriodServicePurchases:   2,
			PeriodNewKeyPurchases:    1,
			PeriodExtensionPurchases: 1,
		}},
		RefundRows: []database.RefundPeriodRow{{
			PeriodStart: "2026-07-12",
			Currency:    "MMK",
			RefundTotal: 100,
			RefundCount: 1,
		}},
		RangeUniqueCustomers: 2,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Current.GrossServiceRevenue != 1000 {
		t.Fatalf("gross=%v", report.Current.GrossServiceRevenue)
	}
	if report.Current.Refunds != 100 {
		t.Fatalf("refunds=%v", report.Current.Refunds)
	}
	if report.Current.NetServiceRevenue != 900 {
		t.Fatalf("net=%v", report.Current.NetServiceRevenue)
	}
	if report.Current.UniqueCustomers != 2 {
		t.Fatalf("customers=%d", report.Current.UniqueCustomers)
	}
	if report.Current.AverageOrderValue != 500 {
		t.Fatalf("aov=%v", report.Current.AverageOrderValue)
	}
	if !report.InProgress {
		t.Fatal("current day must be in progress")
	}
	if report.Delta != nil && report.Prior == nil {
		t.Fatal("delta requires prior")
	}
}

func TestBuildFinanceReport_PriorDeltaAndTrendOrder(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, loc)
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodDay,
		Now:          now,
		CurrentStart: time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		CurrentEnd:   time.Date(2026, 7, 13, 0, 0, 0, 0, loc),
		PriorStart:   time.Date(2026, 7, 10, 0, 0, 0, 0, loc),
		PriorEnd:     time.Date(2026, 7, 11, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-11", Currency: "MMK", PeriodServiceRevenue: 500, PeriodServicePurchases: 1, ServiceRevenue: 500, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
			{PeriodStart: "2026-07-12", Currency: "MMK", PeriodServiceRevenue: 1000, PeriodServicePurchases: 2, ServiceRevenue: 1000, TotalPurchases: 2, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		RangeUniqueCustomers: 2,
		PriorPurchaseRows: []database.RevenueSummaryRow{
			{PeriodStart: "2026-07-10", Currency: "MMK", PeriodServiceRevenue: 500, PeriodServicePurchases: 1, ServiceRevenue: 500, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key"},
		},
		PriorUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.Prior == nil || report.Prior.GrossServiceRevenue != 500 {
		t.Fatalf("prior=%+v", report.Prior)
	}
	if report.Delta == nil || report.Delta.GrossServiceRevenue.Absolute != 500 {
		t.Fatalf("delta=%+v", report.Delta)
	}
	if len(report.Trend) < 2 {
		t.Fatalf("trend len=%d", len(report.Trend))
	}
	if report.Trend[0].PeriodStart > report.Trend[1].PeriodStart {
		t.Fatal("trend must be ascending")
	}
}

func TestBuildFinanceReport_CustomInclusiveRange(t *testing.T) {
	loc := YangonLocation()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, loc)
	in := BuildFinanceReportInput{
		Period:       database.RevenuePeriodCustom,
		Now:          time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
		CustomFrom:   &from,
		CustomTo:     &to,
		CurrentStart: from,
		CurrentEnd:   time.Date(2026, 2, 1, 0, 0, 0, 0, loc),
		PurchaseRows: []database.RevenueSummaryRow{{
			PeriodStart: "2026-01-15", Currency: "MMK",
			PeriodServiceRevenue: 100, PeriodServicePurchases: 1,
			ServiceRevenue: 100, TotalPurchases: 1, PaymentMethod: "kbz", RevenueCategory: "new_key",
		}},
		RangeUniqueCustomers: 1,
	}
	report, err := BuildFinanceReport(in)
	if err != nil {
		t.Fatal(err)
	}
	if report.RangeStart != "2026-01-01" || report.RangeEnd != "2026-01-31" {
		t.Fatalf("range %s..%s", report.RangeStart, report.RangeEnd)
	}
	if report.InProgress {
		t.Fatal("historical custom range must not be in progress")
	}
}
```

- [ ] **Step 2: Run tests expecting fail**

Run: `go test ./internal/reporting/ -run TestBuildFinanceReport -count=1`
Expected: FAIL (`BuildFinanceReport` undefined).

- [ ] **Step 3: Implement `finance.go`**

Create `internal/reporting/finance.go` with the contract types from Shared interface contracts (`MoneyDelta`, `FinanceMetrics`, `FinanceDelta`, `CategoryBreakdown`, `MethodBreakdown`, `FinanceTrendBucket`, `FinanceReport`, `BuildFinanceReportInput`) plus:

```go
package reporting

import (
	"fmt"
	"sort"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func moneyDelta(current, prior float64) MoneyDelta {
	d := MoneyDelta{Absolute: RoundMoney(current - prior)}
	if prior == 0 {
		d.Percentage = nil
		return d
	}
	p := RoundMoney(((current - prior) / prior) * 100)
	d.Percentage = &p
	return d
}

func finalizeMetrics(m FinanceMetrics, uniqueCustomers int) FinanceMetrics {
	m.GrossServiceRevenue = RoundMoney(m.GrossServiceRevenue)
	m.Refunds = RoundMoney(m.Refunds)
	m.NetServiceRevenue = RoundMoney(m.GrossServiceRevenue - m.Refunds)
	m.CashCollected = RoundMoney(m.CashCollected)
	m.WalletTopUps = RoundMoney(m.WalletTopUps)
	m.WalletSpend = RoundMoney(m.WalletSpend)
	m.UniqueCustomers = uniqueCustomers
	if m.SuccessfulOrders > 0 {
		m.AverageOrderValue = RoundMoney(m.GrossServiceRevenue / float64(m.SuccessfulOrders))
	} else {
		m.AverageOrderValue = 0
	}
	return m
}

type bucketKey struct {
	start    string
	currency string
}

func aggregatePurchaseBuckets(rows []database.RevenueSummaryRow) map[bucketKey]FinanceMetrics {
	seenPeriod := map[bucketKey]bool{}
	out := map[bucketKey]FinanceMetrics{}
	for _, row := range rows {
		currency := firstNonEmpty(row.Currency, "MMK")
		start := firstNonEmpty(row.PeriodStart, row.Day)
		key := bucketKey{start: start, currency: currency}
		m := out[key]
		if !seenPeriod[key] {
			seenPeriod[key] = true
			m.GrossServiceRevenue += row.PeriodServiceRevenue
			m.CashCollected += row.PeriodCashCollected
			m.WalletTopUps += row.PeriodWalletTopUps
			m.WalletSpend += row.PeriodWalletSpend
			m.SuccessfulOrders += row.PeriodServicePurchases
			m.NewSubscriptions += row.PeriodNewKeyPurchases
			m.Extensions += row.PeriodExtensionPurchases
		}
		out[key] = m
	}
	return out
}

func applyRefunds(buckets map[bucketKey]FinanceMetrics, refunds []database.RefundPeriodRow) {
	for _, rr := range refunds {
		key := bucketKey{start: rr.PeriodStart, currency: firstNonEmpty(rr.Currency, "MMK")}
		m := buckets[key]
		m.Refunds += rr.RefundTotal
		buckets[key] = m
	}
}

func sumMetrics(buckets map[bucketKey]FinanceMetrics) FinanceMetrics {
	var total FinanceMetrics
	for _, m := range buckets {
		total.GrossServiceRevenue += m.GrossServiceRevenue
		total.Refunds += m.Refunds
		total.CashCollected += m.CashCollected
		total.WalletTopUps += m.WalletTopUps
		total.WalletSpend += m.WalletSpend
		total.SuccessfulOrders += m.SuccessfulOrders
		total.NewSubscriptions += m.NewSubscriptions
		total.Extensions += m.Extensions
	}
	return total
}

func bucketInclusiveEnd(period database.RevenueSummaryPeriod, start time.Time) time.Time {
	switch period {
	case database.RevenuePeriodWeek:
		return start.AddDate(0, 0, 6)
	case database.RevenuePeriodMonth:
		return start.AddDate(0, 1, -1)
	case database.RevenuePeriodYear:
		return start.AddDate(1, 0, -1)
	default: // day, custom day buckets
		return start
	}
}

func BuildFinanceReport(in BuildFinanceReportInput) (FinanceReport, error) {
	if in.CurrentStart.IsZero() || in.CurrentEnd.IsZero() || !in.CurrentEnd.After(in.CurrentStart) {
		return FinanceReport{}, fmt.Errorf("current window is required")
	}
	loc := YangonLocation()
	now := in.Now.In(loc)
	period := in.Period
	if period == "" {
		period = database.RevenuePeriodDay
	}

	currentBuckets := aggregatePurchaseBuckets(in.PurchaseRows)
	applyRefunds(currentBuckets, in.RefundRows)
	priorBuckets := aggregatePurchaseBuckets(in.PriorPurchaseRows)
	applyRefunds(priorBuckets, in.PriorRefundRows)

	current := finalizeMetrics(sumMetrics(currentBuckets), in.RangeUniqueCustomers)
	prior := finalizeMetrics(sumMetrics(priorBuckets), in.PriorUniqueCustomers)

	// Range-level categories/methods (selected window totals).
	categories := buildCategoryBreakdown(in.PurchaseRows)
	methods := buildMethodBreakdown(in.PurchaseRows)

	// Per-period breakdown maps for truthful expandable trend rows.
	catsByPeriod := map[string][]database.RevenueSummaryRow{}
	for _, row := range in.PurchaseRows {
		start := firstNonEmpty(row.PeriodStart, row.Day)
		catsByPeriod[start] = append(catsByPeriod[start], row)
	}

	// trend ascending
	starts := make([]string, 0, len(currentBuckets))
	for k := range currentBuckets {
		starts = append(starts, k.start)
	}
	sort.Strings(starts)
	// unique starts
	uniqStarts := make([]string, 0, len(starts))
	seenStart := map[string]bool{}
	for _, s := range starts {
		if seenStart[s] {
			continue
		}
		seenStart[s] = true
		uniqStarts = append(uniqStarts, s)
	}
	trend := make([]FinanceTrendBucket, 0, len(uniqStarts))
	for _, startStr := range uniqStarts {
		// Use MMK bucket when present; otherwise the first currency for that start.
		var m FinanceMetrics
		found := false
		for k, v := range currentBuckets {
			if k.start != startStr {
				continue
			}
			if !found || k.currency == "MMK" {
				m = v
				found = true
				if k.currency == "MMK" {
					break
				}
			}
		}
		startDay, _ := time.ParseInLocation("2006-01-02", startStr, loc)
		endDay := bucketInclusiveEnd(period, startDay)
		bucketStart := startDay
		bucketEndExclusive := endDay.AddDate(0, 0, 1)
		inProgress := !now.Before(bucketStart) && now.Before(bucketEndExclusive)
		periodRows := catsByPeriod[startStr]
		trend = append(trend, FinanceTrendBucket{
			PeriodStart: startStr,
			PeriodEnd:   endDay.Format("2006-01-02"),
			InProgress:  inProgress,
			Metrics:     finalizeMetrics(m, 0),
			Categories:  buildCategoryBreakdown(periodRows),
			Methods:     buildMethodBreakdown(periodRows),
		})
	}

	rangeStart := in.CurrentStart.In(loc).Format("2006-01-02")
	rangeEnd := in.CurrentEnd.In(loc).Add(-time.Nanosecond).Format("2006-01-02")
	inProgress := !now.Before(in.CurrentStart) && now.Before(in.CurrentEnd)

	var priorPtr *FinanceMetrics
	var deltaPtr *FinanceDelta
	if !in.PriorStart.IsZero() && in.PriorEnd.After(in.PriorStart) {
		p := prior
		priorPtr = &p
		deltaPtr = &FinanceDelta{
			GrossServiceRevenue: moneyDelta(current.GrossServiceRevenue, prior.GrossServiceRevenue),
			Refunds:             moneyDelta(current.Refunds, prior.Refunds),
			NetServiceRevenue:   moneyDelta(current.NetServiceRevenue, prior.NetServiceRevenue),
			CashCollected:       moneyDelta(current.CashCollected, prior.CashCollected),
		}
	}

	return FinanceReport{
		Period:      string(period),
		Timezone:    "Asia/Yangon",
		Currency:    firstNonEmpty(currentCurrency(in.PurchaseRows), "MMK"),
		RangeStart:  rangeStart,
		RangeEnd:    rangeEnd,
		GeneratedAt: now,
		InProgress:  inProgress,
		Current:     current,
		Prior:       priorPtr,
		Delta:       deltaPtr,
		Categories:  categories,
		Methods:     methods,
		Trend:       trend,
	}, nil
}

func currentCurrency(rows []database.RevenueSummaryRow) string {
	for _, row := range rows {
		if row.Currency != "" {
			return row.Currency
		}
	}
	return "MMK"
}

func buildCategoryBreakdown(rows []database.RevenueSummaryRow) []CategoryBreakdown {
	catMap := map[string]CategoryBreakdown{}
	for _, row := range rows {
		cat := firstNonEmpty(row.RevenueCategory, "unknown")
		c := catMap[cat]
		c.Category = cat
		c.Orders += row.TotalPurchases
		if row.InvoiceType == string(database.InvoiceTypeWalletTopUp) || cat == "wallet_topup" {
			c.Amount += row.WalletTopUps
		} else {
			c.Amount += row.ServiceRevenue
		}
		catMap[cat] = c
	}
	out := make([]CategoryBreakdown, 0, len(catMap))
	for _, c := range catMap {
		c.Amount = RoundMoney(c.Amount)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Category < out[j].Category })
	return out
}

func buildMethodBreakdown(rows []database.RevenueSummaryRow) []MethodBreakdown {
	methodMap := map[string]MethodBreakdown{}
	for _, row := range rows {
		method := firstNonEmpty(row.PaymentMethod, "unknown")
		m := methodMap[method]
		m.Method = method
		m.Transactions += row.TotalPurchases
		m.ServiceRevenue += row.ServiceRevenue
		m.CashCollected += row.CashCollected
		m.WalletTopUps += row.WalletTopUps
		m.WalletSpend += row.WalletSpend
		methodMap[method] = m
	}
	out := make([]MethodBreakdown, 0, len(methodMap))
	for _, m := range methodMap {
		m.ServiceRevenue = RoundMoney(m.ServiceRevenue)
		m.CashCollected = RoundMoney(m.CashCollected)
		m.WalletTopUps = RoundMoney(m.WalletTopUps)
		m.WalletSpend = RoundMoney(m.WalletSpend)
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ServiceRevenue == out[j].ServiceRevenue {
			return out[i].Method < out[j].Method
		}
		return out[i].ServiceRevenue > out[j].ServiceRevenue
	})
	return out
}
```

- [ ] **Step 4: Run tests expecting pass**

Run: `go test ./internal/reporting/ -run TestBuildFinanceReport -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/reporting/finance.go internal/reporting/finance_test.go
git commit -m "feat(reporting): FinanceReport builder with net, prior delta, and trend"
```

---

### Task 6: CSV export from the same FinanceReport DTO

**Files:**
- Create: `internal/reporting/finance_csv.go`
- Modify: `internal/reporting/finance_test.go`

**Consumes:** `FinanceReport`.  
**Produces:** `FormatFinanceReportCSV(report) ([]byte, error)`.

- [ ] **Step 1: Write failing CSV parity test**

```go
func TestFormatFinanceReportCSV_MatchesJSONTotals(t *testing.T) {
	report := FinanceReport{
		Period: "day", Timezone: "Asia/Yangon", Currency: "MMK",
		RangeStart: "2026-07-12", RangeEnd: "2026-07-12",
		Current: FinanceMetrics{
			GrossServiceRevenue: 1000, Refunds: 100, NetServiceRevenue: 900,
			CashCollected: 800, WalletTopUps: 200, WalletSpend: 200,
			SuccessfulOrders: 2, UniqueCustomers: 2, AverageOrderValue: 500,
			NewSubscriptions: 1, Extensions: 1,
		},
		Methods: []MethodBreakdown{{Method: "kbz", Transactions: 1, ServiceRevenue: 600, CashCollected: 600}},
		Categories: []CategoryBreakdown{{Category: "new_key", Orders: 1, Amount: 600}},
		Trend: []FinanceTrendBucket{{
			PeriodStart: "2026-07-12", PeriodEnd: "2026-07-12",
			Metrics:     FinanceMetrics{GrossServiceRevenue: 1000, Refunds: 100, NetServiceRevenue: 900},
			Categories:  []CategoryBreakdown{{Category: "new_key", Orders: 1, Amount: 600}},
			Methods:     []MethodBreakdown{{Method: "kbz", Transactions: 1, ServiceRevenue: 600, CashCollected: 600}},
		}},
	}
	csvBytes, err := FormatFinanceReportCSV(report)
	if err != nil {
		t.Fatal(err)
	}
	s := string(csvBytes)
	for _, want := range []string{
		"gross_service_revenue,1000.00",
		"refunds,100.00",
		"net_service_revenue,900.00",
		"cash_collected,800.00",
		"unique_customers,2",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("csv missing %q in %s", want, s)
		}
	}
}
```

- [ ] **Step 2: Run test expecting fail**

Run: `go test ./internal/reporting/ -run TestFormatFinanceReportCSV_MatchesJSONTotals -count=1`
Expected: FAIL undefined `FormatFinanceReportCSV`.

- [ ] **Step 3: Implement CSV encoder**

```go
// internal/reporting/finance_csv.go
package reporting

import (
	"bytes"
	"encoding/csv"
	"fmt"
)

func FormatFinanceReportCSV(report FinanceReport) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"section", "key", "value"}); err != nil {
		return nil, err
	}
	rows := [][]string{
		{"meta", "period", report.Period},
		{"meta", "timezone", report.Timezone},
		{"meta", "currency", report.Currency},
		{"meta", "range_start", report.RangeStart},
		{"meta", "range_end", report.RangeEnd},
		{"meta", "in_progress", fmt.Sprintf("%t", report.InProgress)},
		{"current", "gross_service_revenue", fmt.Sprintf("%.2f", RoundMoney(report.Current.GrossServiceRevenue))},
		{"current", "refunds", fmt.Sprintf("%.2f", RoundMoney(report.Current.Refunds))},
		{"current", "net_service_revenue", fmt.Sprintf("%.2f", RoundMoney(report.Current.NetServiceRevenue))},
		{"current", "cash_collected", fmt.Sprintf("%.2f", RoundMoney(report.Current.CashCollected))},
		{"current", "wallet_topups", fmt.Sprintf("%.2f", RoundMoney(report.Current.WalletTopUps))},
		{"current", "wallet_spend", fmt.Sprintf("%.2f", RoundMoney(report.Current.WalletSpend))},
		{"current", "successful_orders", fmt.Sprintf("%d", report.Current.SuccessfulOrders)},
		{"current", "unique_customers", fmt.Sprintf("%d", report.Current.UniqueCustomers)},
		{"current", "average_order_value", fmt.Sprintf("%.2f", RoundMoney(report.Current.AverageOrderValue))},
		{"current", "new_subscriptions", fmt.Sprintf("%d", report.Current.NewSubscriptions)},
		{"current", "extensions", fmt.Sprintf("%d", report.Current.Extensions)},
	}
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	for _, c := range report.Categories {
		if err := w.Write([]string{"category", c.Category, fmt.Sprintf("%d:%.2f", c.Orders, RoundMoney(c.Amount))}); err != nil {
			return nil, err
		}
	}
	for _, m := range report.Methods {
		if err := w.Write([]string{"method", m.Method, fmt.Sprintf("%d:%.2f:%.2f", m.Transactions, RoundMoney(m.ServiceRevenue), RoundMoney(m.CashCollected))}); err != nil {
			return nil, err
		}
	}
	for _, tr := range report.Trend {
		if err := w.Write([]string{"trend", tr.PeriodStart, fmt.Sprintf("%.2f:%.2f:%.2f", RoundMoney(tr.Metrics.GrossServiceRevenue), RoundMoney(tr.Metrics.Refunds), RoundMoney(tr.Metrics.NetServiceRevenue))}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
```

- [ ] **Step 4: Run test expecting pass**

Run: `go test ./internal/reporting/ -run TestFormatFinanceReportCSV -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/reporting/finance_csv.go internal/reporting/finance_test.go
git commit -m "feat(reporting): CSV export from shared FinanceReport DTO"
```

---

### Task 7: Report window resolver + distinct customers + FinanceService

**Files:**
- Create: `internal/reporting/ranges.go`
- Create: `internal/reporting/ranges_test.go`
- Create: `internal/reporting/service.go`
- Create: `internal/reporting/service_test.go`
- Modify: `internal/database/purchase.go`
- Modify: `internal/database/purchase_revenue_test.go`

**Consumes:** purchase range query, refund sum, `BuildFinanceReport`.  
**Produces:** `ResolveReportWindow`, `CountDistinctServiceCustomers`, `FinanceService.GetReport`.

**Locked assembly path:** only `FinanceService.GetReport` loads DB rows and calls `BuildFinanceReport`. API/Telegram/cron call this service. `internal/payment` is not involved.

- [ ] **Step 1: Write failing tests**

```go
// internal/database/purchase_revenue_test.go
func TestBuildCountDistinctServiceCustomersSQL_ExcludesAdminAndWalletTopups(t *testing.T) {
	q := buildCountDistinctServiceCustomersSQL()
	for _, want := range []string{
		"COUNT(DISTINCT p.customer_id)",
		"invoice_type <> 'wallet_topup'",
		"status = 'paid'",
		"telegram_id",
		"paid_at >= $1",
		"paid_at < $2",
	} {
		if !strings.Contains(q, want) {
			t.Fatalf("missing %q: %s", want, q)
		}
	}
}
```

```go
// internal/reporting/ranges_test.go
package reporting

import (
	"errors"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

func TestResolveReportWindow_DayDefaultsAndPrior(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 15, 0, 0, 0, loc)
	cs, ce, ps, pe, err := ResolveReportWindow(database.RevenuePeriodDay, now, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// default historyPeriods=30; end = startOfDay(now)+1day; start = end-30d
	wantEnd := time.Date(2026, 7, 13, 0, 0, 0, 0, loc)
	wantStart := wantEnd.AddDate(0, 0, -30) // 2026-06-13
	if !cs.Equal(wantStart) || !ce.Equal(wantEnd) {
		t.Fatalf("current window got %v..%v want %v..%v", cs, ce, wantStart, wantEnd)
	}
	if !pe.Equal(wantStart) {
		t.Fatalf("prior end=%v want %v", pe, wantStart)
	}
	if !ps.Equal(wantStart.AddDate(0, 0, -30)) {
		t.Fatalf("prior start=%v want %v", ps, wantStart.AddDate(0, 0, -30))
	}
}

func TestResolveReportWindow_CustomMax366(t *testing.T) {
	loc := YangonLocation()
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, loc)
	to := time.Date(2026, 1, 3, 0, 0, 0, 0, loc) // > 366 days inclusive
	_, _, _, _, err := ResolveReportWindow(database.RevenuePeriodCustom, time.Now().In(loc), 0, &from, &to)
	if err == nil {
		t.Fatal("expected over-bound custom range error")
	}
	if !errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v want ErrInvalidReportQuery", err)
	}
}

func TestResolveReportWindow_PeriodsOverMax(t *testing.T) {
	loc := YangonLocation()
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, loc)
	_, _, _, _, err := ResolveReportWindow(database.RevenuePeriodDay, now, 9999, nil, nil)
	if !errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v", err)
	}
}
```

```go
// internal/reporting/service_test.go
package reporting

import (
	"context"
	"errors"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type fakePurchases struct {
	rows []database.RevenueSummaryRow
	n    int
	err  error
}

func (f *fakePurchases) GetRevenueSummaryRange(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod) ([]database.RevenueSummaryRow, error) {
	return f.rows, f.err
}
func (f *fakePurchases) CountDistinctServiceCustomers(ctx context.Context, start, end time.Time) (int, error) {
	return f.n, f.err
}

type fakeRefunds struct {
	rows []database.RefundPeriodRow
	err  error
}

func (f *fakeRefunds) SumRefundsByPeriod(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod, adminTelegramID int64) ([]database.RefundPeriodRow, error) {
	return f.rows, f.err
}

func TestFinanceService_GetReport_InvalidPeriod(t *testing.T) {
	s := NewFinanceService(&fakePurchases{}, &fakeRefunds{})
	_, err := s.GetReport(context.Background(), ReportQuery{Period: database.RevenueSummaryPeriod("quarter")})
	if !errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v", err)
	}
}

func TestFinanceService_GetReport_RepoErrorIsNotValidation(t *testing.T) {
	s := NewFinanceService(&fakePurchases{err: errors.New("db down")}, &fakeRefunds{})
	loc := YangonLocation()
	_, err := s.GetReport(context.Background(), ReportQuery{
		Period: database.RevenuePeriodDay,
		Now:    time.Date(2026, 7, 12, 0, 0, 0, 0, loc),
	})
	if err == nil || errors.Is(err, ErrInvalidReportQuery) {
		t.Fatalf("err=%v want non-validation repo error", err)
	}
}
```

- [ ] **Step 2: Run expecting fail**

Run: `go test ./internal/database/ -run TestBuildCountDistinctServiceCustomersSQL -count=1 && go test ./internal/reporting/ -run 'TestResolveReportWindow|TestFinanceService' -count=1`
Expected: FAIL undefined symbols.

- [ ] **Step 3: Implement count SQL, ranges, and FinanceService**

```go
// in internal/database/purchase.go
func buildCountDistinctServiceCustomersSQL() string {
	return `
		SELECT COUNT(DISTINCT p.customer_id)
		FROM purchase p
		JOIN customer c ON c.id = p.customer_id
		WHERE p.status = 'paid'
		  AND p.paid_at IS NOT NULL
		  AND p.invoice_type <> 'wallet_topup'
		  AND p.paid_at >= $1
		  AND p.paid_at < $2
		  AND ($3::bigint = 0 OR c.telegram_id <> $3)`
}

func (pr *PurchaseRepository) CountDistinctServiceCustomers(ctx context.Context, start, end time.Time) (int, error) {
	adminID := config.GetAdminTelegramId()
	var n int
	err := pr.pool.QueryRow(ctx, buildCountDistinctServiceCustomersSQL(), start, end, adminID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count distinct service customers: %w", err)
	}
	return n, nil
}
```

```go
// internal/reporting/ranges.go
package reporting

import (
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

var ErrInvalidReportQuery = fmt.Errorf("invalid report query")

const (
	defaultHistoryDay   = 30
	defaultHistoryWeek  = 12
	defaultHistoryMonth = 12
	defaultHistoryYear  = 5
	maxHistoryDay       = 366
	maxHistoryWeek      = 104
	maxHistoryMonth     = 120
	maxHistoryYear      = 20
	maxCustomInclusive  = 366
)

func ResolveReportWindow(period database.RevenueSummaryPeriod, now time.Time, historyPeriods int, customFrom, customTo *time.Time) (time.Time, time.Time, time.Time, time.Time, error) {
	loc := YangonLocation()
	now = now.In(loc)
	switch period {
	case database.RevenuePeriodDay:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryDay
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryDay {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for day", ErrInvalidReportQuery, maxHistoryDay)
		}
		end := StartOfDay(now).AddDate(0, 0, 1)
		start := end.AddDate(0, 0, -historyPeriods)
		priorEnd := start
		priorStart := priorEnd.AddDate(0, 0, -historyPeriods)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodWeek:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryWeek
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryWeek {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for week", ErrInvalidReportQuery, maxHistoryWeek)
		}
		end := StartOfWeek(now).AddDate(0, 0, 7)
		start := end.AddDate(0, 0, -7*historyPeriods)
		priorEnd := start
		priorStart := priorEnd.AddDate(0, 0, -7*historyPeriods)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodMonth:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryMonth
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryMonth {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for month", ErrInvalidReportQuery, maxHistoryMonth)
		}
		end := StartOfMonth(now).AddDate(0, 1, 0)
		start := end.AddDate(0, -historyPeriods, 0)
		priorEnd := start
		priorStart := priorEnd.AddDate(0, -historyPeriods, 0)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodYear:
		if historyPeriods == 0 {
			historyPeriods = defaultHistoryYear
		}
		if historyPeriods < 1 || historyPeriods > maxHistoryYear {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: periods must be 1..%d for year", ErrInvalidReportQuery, maxHistoryYear)
		}
		end := StartOfYear(now).AddDate(1, 0, 0)
		start := end.AddDate(-historyPeriods, 0, 0)
		priorEnd := start
		priorStart := priorEnd.AddDate(-historyPeriods, 0, 0)
		return start, end, priorStart, priorEnd, nil
	case database.RevenuePeriodCustom:
		if customFrom == nil || customTo == nil {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: custom requires from and to", ErrInvalidReportQuery)
		}
		start, end, err := database.InclusiveYangonDateRangeToHalfOpen(*customFrom, *customTo)
		if err != nil {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: %v", ErrInvalidReportQuery, err)
		}
		inclusiveDays := int(end.Sub(start).Hours()/24 + 0.5)
		if inclusiveDays < 1 || inclusiveDays > maxCustomInclusive {
			return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: custom range must be 1..%d days", ErrInvalidReportQuery, maxCustomInclusive)
		}
		priorEnd := start
		priorStart := priorEnd.AddDate(0, 0, -inclusiveDays)
		return start, end, priorStart, priorEnd, nil
	default:
		return time.Time{}, time.Time{}, time.Time{}, time.Time{}, fmt.Errorf("%w: unsupported period %s", ErrInvalidReportQuery, period)
	}
}
```

```go
// internal/reporting/service.go
package reporting

import (
	"context"
	"fmt"
	"time"

	"remnawave-tg-shop-bot/internal/config"
	"remnawave-tg-shop-bot/internal/database"
)

type purchaseRevenueReader interface {
	GetRevenueSummaryRange(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod) ([]database.RevenueSummaryRow, error)
	CountDistinctServiceCustomers(ctx context.Context, start, end time.Time) (int, error)
}

type refundPeriodReader interface {
	SumRefundsByPeriod(ctx context.Context, start, end time.Time, period database.RevenueSummaryPeriod, adminTelegramID int64) ([]database.RefundPeriodRow, error)
}

type FinanceService struct {
	purchases purchaseRevenueReader
	refunds   refundPeriodReader
	adminID   func() int64
	now       func() time.Time
}

func NewFinanceService(purchases purchaseRevenueReader, refunds refundPeriodReader) *FinanceService {
	return &FinanceService{
		purchases: purchases,
		refunds:   refunds,
		adminID:   config.GetAdminTelegramId,
		now:       time.Now,
	}
}

type ReportQuery struct {
	Period         database.RevenueSummaryPeriod
	HistoryPeriods int
	CustomFrom     *time.Time
	CustomTo       *time.Time
	Now            time.Time
}

func (s *FinanceService) GetReport(ctx context.Context, q ReportQuery) (FinanceReport, error) {
	now := q.Now
	if now.IsZero() {
		now = s.now()
	}
	period := q.Period
	if period == "" {
		period = database.RevenuePeriodDay
	}
	// Reject unknown periods early (Normalize already used by API; still guard).
	switch period {
	case database.RevenuePeriodDay, database.RevenuePeriodWeek, database.RevenuePeriodMonth, database.RevenuePeriodYear, database.RevenuePeriodCustom:
	default:
		return FinanceReport{}, fmt.Errorf("%w: unsupported period %s", ErrInvalidReportQuery, period)
	}

	bucketPeriod := period
	if period == database.RevenuePeriodCustom {
		bucketPeriod = database.RevenuePeriodDay
	}

	cs, ce, ps, pe, err := ResolveReportWindow(period, now, q.HistoryPeriods, q.CustomFrom, q.CustomTo)
	if err != nil {
		return FinanceReport{}, err
	}

	purchaseRows, err := s.purchases.GetRevenueSummaryRange(ctx, cs, ce, bucketPeriod)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load purchase revenue: %w", err)
	}
	priorPurchaseRows, err := s.purchases.GetRevenueSummaryRange(ctx, ps, pe, bucketPeriod)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load prior purchase revenue: %w", err)
	}
	adminID := s.adminID()
	refundRows, err := s.refunds.SumRefundsByPeriod(ctx, cs, ce, bucketPeriod, adminID)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load refunds: %w", err)
	}
	priorRefundRows, err := s.refunds.SumRefundsByPeriod(ctx, ps, pe, bucketPeriod, adminID)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("load prior refunds: %w", err)
	}
	uniq, err := s.purchases.CountDistinctServiceCustomers(ctx, cs, ce)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("count customers: %w", err)
	}
	priorUniq, err := s.purchases.CountDistinctServiceCustomers(ctx, ps, pe)
	if err != nil {
		return FinanceReport{}, fmt.Errorf("count prior customers: %w", err)
	}

	return BuildFinanceReport(BuildFinanceReportInput{
		Period:               period,
		Now:                  now.In(YangonLocation()),
		HistoryPeriods:       q.HistoryPeriods,
		CustomFrom:           q.CustomFrom,
		CustomTo:             q.CustomTo,
		PurchaseRows:         purchaseRows,
		RefundRows:           refundRows,
		PriorPurchaseRows:    priorPurchaseRows,
		PriorRefundRows:      priorRefundRows,
		RangeUniqueCustomers: uniq,
		PriorUniqueCustomers: priorUniq,
		CurrentStart:         cs,
		CurrentEnd:           ce,
		PriorStart:           ps,
		PriorEnd:             pe,
	})
}
```

- [ ] **Step 4: Run tests pass**

Run: `go test ./internal/database/ ./internal/reporting/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add \
  internal/database/purchase.go \
  internal/database/purchase_revenue_test.go \
  internal/reporting/ranges.go \
  internal/reporting/ranges_test.go \
  internal/reporting/service.go \
  internal/reporting/service_test.go
git commit -m "feat(reporting): FinanceService assembly with windows and unique customers"
```

---

### Task 8: Admin financial adjustment creation API

**Files:**
- Modify: `internal/api/handlers.go`
- Modify: `internal/api/server.go`
- Modify: `cmd/app/main.go`
- Create: `internal/api/financial_adjustment_test.go`
- Modify every existing `NewAPIHandler(...)` / `RegisterHandlers(...)` call site in tests to pass the two new trailing nils until Task 9 wires finance service: `nil, nil`.

**Consumes:** `FinancialAdjustmentRepository.Create`.  
**Produces:** `POST /api/admin/financial-adjustments` behind `withAdmin`.

- [ ] **Step 1: Write failing handler tests**

```go
// internal/api/financial_adjustment_test.go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/database"
)

type fakeAdjustmentRepo struct {
	last    database.CreateFinancialAdjustmentInput
	row     *database.FinancialAdjustment
	created bool
	err     error
	calls   int
}

func (f *fakeAdjustmentRepo) Create(ctx context.Context, in database.CreateFinancialAdjustmentInput) (*database.FinancialAdjustment, bool, error) {
	f.calls++
	f.last = in
	if f.err != nil {
		return nil, false, f.err
	}
	if f.row == nil {
		id := int64(1)
		f.row = &database.FinancialAdjustment{
			ID:             id,
			AdjustmentType: in.AdjustmentType,
			Amount:         in.Amount,
			Currency:       in.Currency,
			EffectiveAt:    in.EffectiveAt,
			Reason:         in.Reason,
			ExternalRef:    in.ExternalRef,
			CreatedBy:      in.CreatedBy,
			IdempotencyKey: in.IdempotencyKey,
			CreatedAt:      time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC),
		}
		f.created = true
	}
	return f.row, f.created, nil
}

func TestCreateFinancialAdjustment_MissingIdempotencyKey400(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":100,"currency":"MMK"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if repo.calls != 0 {
		t.Fatal("repo should not be called")
	}
}

func TestCreateFinancialAdjustment_NonPositiveAmount400(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":0,"currency":"MMK","idempotency_key":"k1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestCreateFinancialAdjustment_Created201AndReplay200(t *testing.T) {
	repo := &fakeAdjustmentRepo{}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":100.5,"currency":"MMK","idempotency_key":"refund:1","reason":"test"}`

	req1 := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req1 = req1.WithContext(context.WithValue(req1.Context(), telegramIDKey, int64(42)))
	rec1 := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	if repo.last.CreatedBy != "admin:42" {
		t.Fatalf("created_by=%q", repo.last.CreatedBy)
	}

	repo.created = false // simulate idempotent hit
	req2 := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req2 = req2.WithContext(context.WithValue(req2.Context(), telegramIDKey, int64(42)))
	rec2 := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("replay status=%d", rec2.Code)
	}
	var row database.FinancialAdjustment
	if err := json.Unmarshal(rec2.Body.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	if row.IdempotencyKey != "refund:1" {
		t.Fatalf("key=%q", row.IdempotencyKey)
	}
}

func TestCreateFinancialAdjustment_RepoError500(t *testing.T) {
	repo := &fakeAdjustmentRepo{err: context.DeadlineExceeded}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo)
	body := `{"adjustment_type":"refund","amount":10,"currency":"MMK","idempotency_key":"k2"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.CreateFinancialAdjustment(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestRegisterHandlersProtectsFinancialAdjustmentRoute(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/admin/financial-adjustments", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK || rec.Code == http.StatusCreated {
		t.Fatalf("unauthenticated request must not succeed, status=%d", rec.Code)
	}
}
```

- [ ] **Step 2: Run expecting fail**

Run: `go test ./internal/api/ -run 'TestCreateFinancialAdjustment|TestRegisterHandlersProtectsFinancialAdjustmentRoute' -count=1`
Expected: FAIL (handler/signature/route missing).

- [ ] **Step 3: Implement injection + handler + route**

In `handlers.go` add interface + fields + constructor params:

```go
type financialAdjustmentCreator interface {
	Create(ctx context.Context, in database.CreateFinancialAdjustmentInput) (*database.FinancialAdjustment, bool, error)
}

// on APIHandler:
financeService          *reporting.FinanceService
financialAdjustmentRepo financialAdjustmentCreator

// NewAPIHandler(... existing ..., financeService *reporting.FinanceService, financialAdjustmentRepo financialAdjustmentCreator)
// assign both fields in the returned struct literal.
```

```go
type createFinancialAdjustmentRequest struct {
	PurchaseID     *int64  `json:"purchase_id"`
	AdjustmentType string  `json:"adjustment_type"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
	EffectiveAt    *string `json:"effective_at"`
	Reason         string  `json:"reason"`
	ExternalRef    string  `json:"external_ref"`
	IdempotencyKey string  `json:"idempotency_key"`
}

func (h *APIHandler) CreateFinancialAdjustment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.financialAdjustmentRepo == nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Financial adjustments unavailable", fmt.Errorf("repo nil"))
		return
	}
	telegramID, ok := r.Context().Value(telegramIDKey).(int64)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	var req createFinancialAdjustmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		http.Error(w, "idempotency_key is required", http.StatusBadRequest)
		return
	}
	if req.AdjustmentType != string(database.FinancialAdjustmentTypeRefund) {
		http.Error(w, "adjustment_type must be refund", http.StatusBadRequest)
		return
	}
	if req.Amount <= 0 {
		http.Error(w, "amount must be positive", http.StatusBadRequest)
		return
	}
	effectiveAt := h.currentTime()
	if req.EffectiveAt != nil && strings.TrimSpace(*req.EffectiveAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.EffectiveAt))
		if err != nil {
			http.Error(w, "effective_at must be RFC3339", http.StatusBadRequest)
			return
		}
		effectiveAt = parsed
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "MMK"
	}
	row, created, err := h.financialAdjustmentRepo.Create(r.Context(), database.CreateFinancialAdjustmentInput{
		PurchaseID:     req.PurchaseID,
		AdjustmentType: database.FinancialAdjustmentTypeRefund,
		Amount:         req.Amount,
		Currency:       currency,
		EffectiveAt:    effectiveAt,
		Reason:         req.Reason,
		ExternalRef:    req.ExternalRef,
		CreatedBy:      fmt.Sprintf("admin:%d", telegramID),
		IdempotencyKey: strings.TrimSpace(req.IdempotencyKey),
	})
	if err != nil {
		// validation-style repo errors (amount/key) still possible; map known messages to 400
		msg := err.Error()
		if strings.Contains(msg, "idempotency_key") || strings.Contains(msg, "amount must") || strings.Contains(msg, "unsupported adjustment_type") {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to create financial adjustment", err)
		return
	}
	if created {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(row)
}
```

In `server.go`:

```go
func RegisterHandlers(
	mux *http.ServeMux,
	customerRepo *database.CustomerRepository,
	paymentService *payment.PaymentService,
	telegramBot *bot.Bot,
	tm *translation.Manager,
	subKeyRepo *database.SubscriptionKeyRepository,
	promoCodeRepository *database.PromoCodeRepository,
	walletService *walletsvc.WalletService,
	referralRepo *database.ReferralRepository,
	appConfigRepo *database.AppConfigRepository,
	financeService *reporting.FinanceService,
	financialAdjustmentRepo *database.FinancialAdjustmentRepository,
) {
	handler := NewAPIHandler(customerRepo, paymentService, telegramBot, tm, subKeyRepo, promoCodeRepository, walletService, referralRepo, appConfigRepo, financeService, financialAdjustmentRepo)
	// existing middleware helpers unchanged
	// existing routes unchanged, plus:
	mux.HandleFunc("/api/admin/financial-adjustments", withAdmin(handler.CreateFinancialAdjustment))
}
```

In `cmd/app/main.go` near DB pool setup:

```go
financialAdjustmentRepository := database.NewFinancialAdjustmentRepository(pool)
financeService := reporting.NewFinanceService(purchaseRepository, financialAdjustmentRepository)
// RegisterHandlers(..., financeService, financialAdjustmentRepository)
```

Update **all** compile-breaking call sites (`server_test.go`, `admin_plans_test.go`, `admin_promos_test.go`, `handlers_test.go`, `trial_test.go`, `support_links.regression_test.go`, `cmd/app/main.go`) to pass the two new trailing arguments.

- [ ] **Step 4: Run tests pass**

Run: `go test ./internal/api/ -run 'TestCreateFinancialAdjustment|TestRegisterHandlersProtectsFinancialAdjustmentRoute' -count=1`
Expected: PASS

Also run: `go test ./internal/api/ -count=1` to catch signature fallout.
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers.go internal/api/server.go internal/api/financial_adjustment_test.go internal/api/server_test.go internal/api/admin_plans_test.go internal/api/admin_promos_test.go internal/api/handlers_test.go internal/api/trial_test.go internal/api/support_links.regression_test.go cmd/app/main.go
git commit -m "feat(api): admin-only financial adjustment create with idempotency"
```

---

### Task 9: Structured GET /api/revenue + export + validation bounds

**Files:**
- Modify: `internal/api/handlers.go`
- Modify: `internal/api/server.go`
- Create: `internal/api/revenue_test.go`

**Consumes:** `FinanceService.GetReport`, `FormatFinanceReportCSV`.  
**Produces:** JSON `FinanceReport`; CSV from same report; 400 vs 500 split.

**Error mapping (locked):**
- Parse/validation failures and `errors.Is(err, reporting.ErrInvalidReportQuery)` → `400` with message body.
- All other `GetReport` errors → `writeSanitizedError(..., 500, "Failed to fetch revenue", err)`.

**Over-bound policy (locked):** return `400` when periods exceed max or custom span > 366 days (no silent clamp).

- [ ] **Step 1: Write failing API tests**

```go
// internal/api/revenue_test.go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"remnawave-tg-shop-bot/internal/reporting"
)

type fakeFinanceService struct {
	report reporting.FinanceReport
	err    error
	last   reporting.ReportQuery
}

// Locked: APIHandler.financeService is financeReporter interface (see handlers.go in this task).
// fakeFinanceService satisfies financeReporter for unit tests.

func (f *fakeFinanceService) GetReport(ctx context.Context, q reporting.ReportQuery) (reporting.FinanceReport, error) {
	f.last = q
	return f.report, f.err
}

func sampleFinanceReport() reporting.FinanceReport {
	metrics := reporting.FinanceMetrics{
		GrossServiceRevenue: 1000,
		Refunds:             100,
		NetServiceRevenue:   900,
		CashCollected:       800,
		SuccessfulOrders:    2,
		UniqueCustomers:     2,
		AverageOrderValue:   500,
	}
	cats := []reporting.CategoryBreakdown{{Category: "new_key", Orders: 1, Amount: 600}}
	methods := []reporting.MethodBreakdown{{
		Method: "kbz", Transactions: 1, ServiceRevenue: 600, CashCollected: 600,
	}}
	return reporting.FinanceReport{
		Period:     "day",
		Timezone:   "Asia/Yangon",
		Currency:   "MMK",
		RangeStart: "2026-07-12",
		RangeEnd:   "2026-07-12",
		InProgress: true,
		Current:    metrics,
		Categories: cats,
		Methods:    methods,
		Trend: []reporting.FinanceTrendBucket{{
			PeriodStart: "2026-07-12",
			PeriodEnd:   "2026-07-12",
			InProgress:  true,
			Metrics:     metrics,
			Categories:  cats,
			Methods:     methods,
		}},
	}
}

func TestGetRevenueSummary_InvalidPeriod400(t *testing.T) {
	// period is validated in parseReportQuery before FinanceService is called
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=quarter", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetRevenueSummary_CustomRequiresFromTo(t *testing.T) {
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=custom&from=2026-01-01", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestGetRevenueSummary_ServiceValidation400(t *testing.T) {
	fake := &fakeFinanceService{err: fmt.Errorf("%w: periods too large", reporting.ErrInvalidReportQuery)}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day&periods=30", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetRevenueSummary_RepoError500(t *testing.T) {
	fake := &fakeFinanceService{err: errors.New("db down")}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestGetRevenueSummary_OKJSON(t *testing.T) {
	fake := &fakeFinanceService{report: sampleFinanceReport()}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake
	req := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day&periods=7", nil)
	req = req.WithContext(context.WithValue(req.Context(), telegramIDKey, int64(42)))
	rec := httptest.NewRecorder()
	h.GetRevenueSummary(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got reporting.FinanceReport
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Current.NetServiceRevenue != 900 {
		t.Fatalf("net=%v", got.Current.NetServiceRevenue)
	}
	if fake.last.HistoryPeriods != 7 {
		t.Fatalf("history=%d", fake.last.HistoryPeriods)
	}
}

func TestExportRevenue_CSVMatchesJSONNet(t *testing.T) {
	rep := sampleFinanceReport()
	fake := &fakeFinanceService{report: rep}
	h := NewAPIHandler(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	h.financeService = fake

	reqJSON := httptest.NewRequest(http.MethodGet, "/api/revenue?period=day", nil)
	reqJSON = reqJSON.WithContext(context.WithValue(reqJSON.Context(), telegramIDKey, int64(42)))
	recJSON := httptest.NewRecorder()
	h.GetRevenueSummary(recJSON, reqJSON)
	var got reporting.FinanceReport
	_ = json.Unmarshal(recJSON.Body.Bytes(), &got)

	reqCSV := httptest.NewRequest(http.MethodGet, "/api/revenue/export?period=day", nil)
	reqCSV = reqCSV.WithContext(context.WithValue(reqCSV.Context(), telegramIDKey, int64(42)))
	recCSV := httptest.NewRecorder()
	h.ExportRevenue(recCSV, reqCSV)
	if recCSV.Code != http.StatusOK {
		t.Fatalf("csv status=%d", recCSV.Code)
	}
	if !strings.Contains(recCSV.Body.String(), "net_service_revenue,900.00") {
		t.Fatalf("csv=%s", recCSV.Body.String())
	}
	if got.Current.NetServiceRevenue != 900 {
		t.Fatalf("json net diverged")
	}
}

func TestRegisterHandlersProtectsRevenueExport(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandlers(mux, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/revenue/export?period=day", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("unauthenticated export must not succeed")
	}
}
```

**Locked field type change for testability:** in `handlers.go` define:

```go
type financeReporter interface {
	GetReport(ctx context.Context, q reporting.ReportQuery) (reporting.FinanceReport, error)
}
```

`APIHandler.financeService` type is `financeReporter`. `NewAPIHandler` still accepts `*reporting.FinanceService` (satisfies interface). Tests assign `*fakeFinanceService`.

Add `fmt` import in revenue_test for the wrapped validation error test.

- [ ] **Step 2: Run expecting fail**

Run: `go test ./internal/api/ -run 'TestGetRevenueSummary|TestExportRevenue|TestRegisterHandlersProtectsRevenueExport' -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement handlers**

```go
func (h *APIHandler) parseReportQuery(r *http.Request) (reporting.ReportQuery, error) {
	period, err := database.NormalizeRevenueSummaryPeriod(r.URL.Query().Get("period"))
	if err != nil {
		return reporting.ReportQuery{}, fmt.Errorf("%w: period must be day, week, month, year, or custom", reporting.ErrInvalidReportQuery)
	}
	q := reporting.ReportQuery{Period: period, Now: h.currentTime()}
	if period == database.RevenuePeriodCustom {
		fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
		toStr := strings.TrimSpace(r.URL.Query().Get("to"))
		if fromStr == "" || toStr == "" {
			return reporting.ReportQuery{}, fmt.Errorf("%w: custom period requires from and to (YYYY-MM-DD)", reporting.ErrInvalidReportQuery)
		}
		loc := reporting.YangonLocation()
		fromDay, err := time.ParseInLocation("2006-01-02", fromStr, loc)
		if err != nil {
			return reporting.ReportQuery{}, fmt.Errorf("%w: from must be YYYY-MM-DD", reporting.ErrInvalidReportQuery)
		}
		toDay, err := time.ParseInLocation("2006-01-02", toStr, loc)
		if err != nil {
			return reporting.ReportQuery{}, fmt.Errorf("%w: to must be YYYY-MM-DD", reporting.ErrInvalidReportQuery)
		}
		q.CustomFrom = &fromDay
		q.CustomTo = &toDay
		return q, nil
	}
	// periods takes precedence; days maps to periods for day period only (compat)
	if p := r.URL.Query().Get("periods"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return reporting.ReportQuery{}, fmt.Errorf("%w: periods must be a positive integer", reporting.ErrInvalidReportQuery)
		}
		q.HistoryPeriods = n
	} else if d := r.URL.Query().Get("days"); d != "" && period == database.RevenuePeriodDay {
		n, err := strconv.Atoi(d)
		if err != nil || n < 1 {
			return reporting.ReportQuery{}, fmt.Errorf("%w: days must be a positive integer", reporting.ErrInvalidReportQuery)
		}
		q.HistoryPeriods = n
	}
	return q, nil
}

func (h *APIHandler) GetRevenueSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.financeService == nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to fetch revenue", fmt.Errorf("finance service nil"))
		return
	}
	q, err := h.parseReportQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	report, err := h.financeService.GetReport(r.Context(), q)
	if err != nil {
		if errors.Is(err, reporting.ErrInvalidReportQuery) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to fetch revenue", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func (h *APIHandler) ExportRevenue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.financeService == nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to export revenue", fmt.Errorf("finance service nil"))
		return
	}
	q, err := h.parseReportQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	report, err := h.financeService.GetReport(r.Context(), q)
	if err != nil {
		if errors.Is(err, reporting.ErrInvalidReportQuery) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to export revenue", err)
		return
	}
	csvBytes, err := reporting.FormatFinanceReportCSV(report)
	if err != nil {
		writeSanitizedError(w, http.StatusInternalServerError, "Failed to export revenue", err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="finance-report.csv"`)
	_, _ = w.Write(csvBytes)
}
```

Register route:

```go
mux.HandleFunc("/api/revenue", withAdmin(handler.GetRevenueSummary))
mux.HandleFunc("/api/revenue/export", withAdmin(handler.ExportRevenue))
```

**Backward compatibility:** response body changes from `[]RevenueSummaryRow` to `FinanceReport`. Document in Task 14.

- [ ] **Step 4: Run tests pass**

Run: `go test ./internal/api/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/handlers.go internal/api/server.go internal/api/revenue_test.go
git commit -m "feat(api): structured finance JSON and CSV export endpoints"
```

---

### Task 10: Telegram `/revenue` + cron shared reporting path

**Files:**
- Modify: `internal/reporting/revenue.go`
- Modify: `internal/reporting/revenue_test.go`
- Modify: `internal/handler/handler.go`
- Modify: `internal/handler/admin.go`
- Modify: `cmd/app/main.go`

**Consumes:** `FinanceService.GetReport`, `FormatTelegramFinanceReport`.  
**Produces:** Telegram text with Net Income / Gross / Refunds / Cash from the same definitions as API.

- [ ] **Step 1: Write failing formatter test**

```go
func TestFormatTelegramFinanceReport_IncludesNetAndRefunds(t *testing.T) {
	report := FinanceReport{
		Period: "day", Currency: "MMK", RangeStart: "2026-07-11", RangeEnd: "2026-07-11",
		Current: FinanceMetrics{
			GrossServiceRevenue: 1000,
			Refunds:             100,
			NetServiceRevenue:   900,
			CashCollected:       800,
			SuccessfulOrders:    2,
			UniqueCustomers:     2,
		},
	}
	text := FormatTelegramFinanceReport("Daily Revenue Report", report)
	for _, want := range []string{"Net Income", "900", "Gross", "1,000", "Refunds", "100", "Cash"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
}

func TestFormatTelegramFinanceReport_InProgressLabel(t *testing.T) {
	report := FinanceReport{
		Period: "day", Currency: "MMK", RangeStart: "2026-07-12", RangeEnd: "2026-07-12",
		InProgress: true,
		Current:    FinanceMetrics{NetServiceRevenue: 0},
	}
	text := FormatTelegramFinanceReport("Daily Revenue Report", report)
	if !strings.Contains(text, "In progress") {
		t.Fatalf("missing In progress: %s", text)
	}
}
```

- [ ] **Step 2: Run expecting fail**

Run: `go test ./internal/reporting/ -run TestFormatTelegramFinanceReport -count=1`
Expected: FAIL.

- [ ] **Step 3: Implement formatter + wire callers**

```go
// internal/reporting/revenue.go
func FormatTelegramFinanceReport(title string, report FinanceReport) string {
	currency := html.EscapeString(firstNonEmpty(report.Currency, "MMK"))
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 <b>%s</b>\n\n", html.EscapeString(title)))
	rangeLabel := report.RangeStart
	if report.RangeStart != report.RangeEnd {
		rangeLabel = fmt.Sprintf("%s to %s", report.RangeStart, report.RangeEnd)
	}
	if report.InProgress {
		rangeLabel += " (In progress)"
	}
	sb.WriteString(fmt.Sprintf("<b>%s</b>\n", html.EscapeString(rangeLabel)))
	sb.WriteString(fmt.Sprintf("Net Income: <b>%s %s</b>\n", FormatNumber(report.Current.NetServiceRevenue), currency))
	sb.WriteString(fmt.Sprintf("Gross: <b>%s %s</b>\n", FormatNumber(report.Current.GrossServiceRevenue), currency))
	sb.WriteString(fmt.Sprintf("Refunds: <b>%s %s</b>\n", FormatNumber(report.Current.Refunds), currency))
	sb.WriteString(fmt.Sprintf("Cash collected: <b>%s %s</b>\n", FormatNumber(report.Current.CashCollected), currency))
	if report.Current.WalletTopUps > 0 || report.Current.WalletSpend > 0 {
		sb.WriteString(fmt.Sprintf("Wallet: %s %s top-ups, %s %s spend\n",
			FormatNumber(report.Current.WalletTopUps), currency,
			FormatNumber(report.Current.WalletSpend), currency))
	}
	sb.WriteString(fmt.Sprintf("Orders: %d plan txns, %d users\n", report.Current.SuccessfulOrders, report.Current.UniqueCustomers))
	sb.WriteString(fmt.Sprintf("Mix: %d new keys, %d extensions\n", report.Current.NewSubscriptions, report.Current.Extensions))
	if len(report.Methods) > 0 {
		sb.WriteString("\n<b>By method</b>\n")
		for _, method := range report.Methods {
			sb.WriteString(fmt.Sprintf("  %s: service %s %s, cash %s %s (%d txns)\n",
				html.EscapeString(method.Method),
				FormatNumber(method.ServiceRevenue), currency,
				FormatNumber(method.CashCollected), currency,
				method.Transactions,
			))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func FormatRevenueCommandFromReport(report FinanceReport, today string) string {
	var sb strings.Builder
	sb.WriteString("📊 <b>Revenue Summary</b>\n\n")
	sb.WriteString(fmt.Sprintf("<b>Selected range</b> %s", report.RangeStart))
	if report.RangeStart != report.RangeEnd {
		sb.WriteString(" to " + report.RangeEnd)
	}
	if report.InProgress {
		sb.WriteString(" (In progress)")
	}
	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Net Income: <b>%s %s</b>\n", FormatNumber(report.Current.NetServiceRevenue), html.EscapeString(report.Currency)))
	sb.WriteString(fmt.Sprintf("Gross: %s, Refunds: %s, Cash: %s\n",
		FormatNumber(report.Current.GrossServiceRevenue),
		FormatNumber(report.Current.Refunds),
		FormatNumber(report.Current.CashCollected),
	))
	sb.WriteString(fmt.Sprintf("Orders: %d, Users: %d\n", report.Current.SuccessfulOrders, report.Current.UniqueCustomers))
	if len(report.Trend) > 0 {
		sb.WriteString("\n<b>Trend</b>\n")
		// show last up to 7 buckets newest last in list already ascending — print reverse for recency
		start := 0
		if len(report.Trend) > 7 {
			start = len(report.Trend) - 7
		}
		for _, b := range report.Trend[start:] {
			label := b.PeriodStart
			if b.PeriodStart == today {
				label += " (today)"
			}
			if b.InProgress {
				label += " *"
			}
			sb.WriteString(fmt.Sprintf("  %s: net %s, gross %s, refunds %s\n",
				html.EscapeString(label),
				FormatNumber(b.Metrics.NetServiceRevenue),
				FormatNumber(b.Metrics.GrossServiceRevenue),
				FormatNumber(b.Metrics.Refunds),
			))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
```

Handler wiring:

```go
// internal/handler/handler.go
// Add field on Handler after mobilePayCache:
financeService *reporting.FinanceService

// Append financeService as the final NewHandler parameter and assign it:
func NewHandler(
	syncService *appSync.SyncService,
	paymentService *payment.PaymentService,
	translation *translation.Manager,
	customerRepository *database.CustomerRepository,
	purchaseRepository *database.PurchaseRepository,
	subscriptionService *notification.SubscriptionService,
	subKeyRepo *database.SubscriptionKeyRepository,
	referralRepository *database.ReferralRepository,
	promoCodeRepository *database.PromoCodeRepository,
	appConfigRepository *database.AppConfigRepository,
	healthcheckService *healthcheck.Service,
	cache *cache.Cache,
	mobilePayCache *cache.Cache,
	financeService *reporting.FinanceService,
) *Handler {
	h := &Handler{
		syncService:         syncService,
		paymentService:      paymentService,
		customerRepository:  customerRepository,
		purchaseRepository:  purchaseRepository,
		translation:         translation,
		subscriptionService: subscriptionService,
		subKeyRepo:          subKeyRepo,
		referralRepository:  referralRepository,
		promoCodeRepository: promoCodeRepository,
		appConfigRepository: appConfigRepository,
		healthcheckService:  healthcheckService,
		cache:               cache,
		mobilePayCache:      mobilePayCache,
		financeService:      financeService,
		limiters:            make(map[int64]*rate.Limiter),
		limitersMu:          &sync.Mutex{},
		adminFlows:          make(map[int64]adminFlowState),
		adminFlowsMu:        &sync.Mutex{},
	}
	go h.cleanupLimiters()
	return h
}
```

```go
// internal/handler/admin.go — replace RevenueCommandHandler body exactly:
func (h Handler) RevenueCommandHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !h.adminOnly(ctx, b, update) {
		return
	}
	if h.financeService == nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "❌ Finance service unavailable",
		})
		return
	}
	report, err := h.financeService.GetReport(ctx, reporting.ReportQuery{
		Period:         database.RevenuePeriodDay,
		HistoryPeriods: 7,
	})
	if err != nil {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   fmt.Sprintf("❌ Error fetching revenue: %v", err),
		})
		return
	}
	today := time.Now().In(reporting.YangonLocation()).Format("2006-01-02")
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    update.Message.Chat.ID,
		Text:      reporting.FormatRevenueCommandFromReport(report, today),
		ParseMode: models.ParseModeHTML,
	})
}
```

Cron in `cmd/app/main.go`:

```go
func sendRevenueReport(ctx context.Context, b *bot.Bot, financeService *reporting.FinanceService, jobName, title string, period database.RevenueSummaryPeriod, start, end time.Time) {
	// Convert half-open [start,end) into inclusive custom query for exact window:
	loc := reporting.YangonLocation()
	from := start.In(loc)
	toInclusive := end.In(loc).Add(-time.Nanosecond)
	toDay := time.Date(toInclusive.Year(), toInclusive.Month(), toInclusive.Day(), 0, 0, 0, 0, loc)
	report, err := financeService.GetReport(ctx, reporting.ReportQuery{
		Period:     database.RevenuePeriodCustom,
		CustomFrom: &from,
		CustomTo:   &toDay,
		Now:        end.Add(-time.Nanosecond),
	})
	if err != nil {
		slog.Error("Revenue report failed", "job", jobName, "error", err)
		return
	}
	// Force period label for display while keeping custom window metrics:
	report.Period = string(period)
	text := reporting.FormatTelegramFinanceReport(title, report)
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:    config.GetAdminTelegramId(),
		Text:      text,
		ParseMode: models.ParseModeHTML,
	}); err != nil {
		slog.Error("Revenue report send failed", "job", jobName, "error", err)
		return
	}
	slog.Info("Revenue report sent", "job", jobName,
		"net", report.Current.NetServiceRevenue,
		"gross", report.Current.GrossServiceRevenue,
		"refunds", report.Current.Refunds,
		"cash", report.Current.CashCollected)
}
```

Update cron call sites to pass `financeService` instead of `purchaseRepository`. Update `NewHandler(...)` call site in `main.go` with `financeService`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/reporting/ ./internal/handler/ -count=1`
Expected: PASS

Run: `go build ./cmd/app`
Expected: success

- [ ] **Step 5: Commit**

```bash
git add internal/reporting/revenue.go internal/reporting/revenue_test.go internal/handler/handler.go internal/handler/admin.go cmd/app/main.go
git commit -m "feat(reporting): route Telegram and cron revenue through FinanceService"
```

---

### Task 11: Frontend finance types + pure helpers

**Files:**
- Create: `web-app/src/lib/finance.ts`
- Create: `web-app/src/lib/finance.test.ts`
- Modify: `web-app/src/lib/translations.ts`

**Consumes:** API JSON contract.  
**Produces:** typed helpers + SVG point builder (display only).

- [ ] **Step 1: Write failing helper tests**

```ts
// web-app/src/lib/finance.test.ts
import { describe, expect, it } from 'vitest';
import {
  formatMoneyMMK,
  formatDelta,
  buildRevenueQuery,
  buildRevenueExportQuery,
  buildTrendPolylinePoints,
} from './finance';

describe('finance helpers', () => {
  it('formats money with two decimals and grouping', () => {
    expect(formatMoneyMMK(900)).toBe('900.00');
    expect(formatMoneyMMK(1234567.5)).toBe('1,234,567.50');
  });

  it('formats delta with percentage or em dash when null', () => {
    expect(formatDelta({ absolute: 100, percentage: 50 })).toContain('+');
    expect(formatDelta({ absolute: -100, percentage: null })).toMatch(/—/);
  });

  it('builds query string for custom range', () => {
    expect(buildRevenueQuery({ period: 'custom', from: '2026-01-01', to: '2026-01-31' }))
      .toBe('/api/revenue?period=custom&from=2026-01-01&to=2026-01-31');
  });

  it('builds export query', () => {
    expect(buildRevenueExportQuery({ period: 'week', periods: 8 }))
      .toBe('/api/revenue/export?period=week&periods=8');
  });

  it('builds svg polyline points', () => {
    const pts = buildTrendPolylinePoints([0, 100, 50], 300, 120, 10);
    expect(pts.split(' ').length).toBe(3);
    expect(pts).toMatch(/^\d+(\.\d+)?,\d+(\.\d+)? /);
  });
});
```

- [ ] **Step 2: Run expecting fail**

Run: `cd web-app && npm test -- src/lib/finance.test.ts`
Expected: FAIL module not found.

- [ ] **Step 3: Implement helpers + translations**

```ts
// web-app/src/lib/finance.ts
export type FinancePeriod = 'day' | 'week' | 'month' | 'year' | 'custom';

export interface MoneyDelta {
  absolute: number;
  percentage: number | null;
}

export interface FinanceMetrics {
  gross_service_revenue: number;
  refunds: number;
  net_service_revenue: number;
  cash_collected: number;
  wallet_topups: number;
  wallet_spend: number;
  successful_orders: number;
  unique_customers: number;
  average_order_value: number;
  new_subscriptions: number;
  extensions: number;
}

export interface FinanceDelta {
  gross_service_revenue: MoneyDelta;
  refunds: MoneyDelta;
  net_service_revenue: MoneyDelta;
  cash_collected: MoneyDelta;
}

export interface CategoryBreakdown {
  category: string;
  orders: number;
  amount: number;
}

export interface MethodBreakdown {
  method: string;
  transactions: number;
  service_revenue: number;
  cash_collected: number;
  wallet_topups: number;
  wallet_spend: number;
}

export interface FinanceTrendBucket {
  period_start: string;
  period_end: string;
  in_progress: boolean;
  metrics: FinanceMetrics;
  categories: CategoryBreakdown[];
  methods: MethodBreakdown[];
}

export interface FinanceReport {
  period: FinancePeriod;
  timezone: string;
  currency: string;
  range_start: string;
  range_end: string;
  generated_at: string;
  in_progress: boolean;
  current: FinanceMetrics;
  prior: FinanceMetrics | null;
  delta: FinanceDelta | null;
  categories: CategoryBreakdown[];
  methods: MethodBreakdown[];
  trend: FinanceTrendBucket[];
}

export function formatMoneyMMK(n: number): string {
  return new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(n);
}

export function formatDelta(d: MoneyDelta): string {
  const sign = d.absolute > 0 ? '+' : '';
  const abs = `${sign}${formatMoneyMMK(d.absolute)}`;
  if (d.percentage === null || Number.isNaN(d.percentage)) {
    return `${abs} (—)`;
  }
  const pSign = d.percentage > 0 ? '+' : '';
  return `${abs} (${pSign}${d.percentage.toFixed(1)}%)`;
}

export function buildRevenueQuery(opts: {
  period: FinancePeriod;
  periods?: number;
  from?: string;
  to?: string;
}): string {
  const q = new URLSearchParams();
  q.set('period', opts.period);
  if (opts.period === 'custom') {
    if (opts.from) q.set('from', opts.from);
    if (opts.to) q.set('to', opts.to);
  } else if (opts.periods !== undefined) {
    q.set('periods', String(opts.periods));
  }
  return `/api/revenue?${q.toString()}`;
}

export function buildRevenueExportQuery(opts: {
  period: FinancePeriod;
  periods?: number;
  from?: string;
  to?: string;
}): string {
  return buildRevenueQuery(opts).replace('/api/revenue?', '/api/revenue/export?');
}

/** Map values to SVG polyline points string for a pure SVG chart. */
export function buildTrendPolylinePoints(
  values: number[],
  width: number,
  height: number,
  pad: number,
): string {
  if (values.length === 0) return '';
  const min = Math.min(...values, 0);
  const max = Math.max(...values, 0);
  const span = max - min || 1;
  const innerW = width - pad * 2;
  const innerH = height - pad * 2;
  return values
    .map((v, i) => {
      const x = pad + (values.length === 1 ? innerW / 2 : (i / (values.length - 1)) * innerW);
      const y = pad + innerH - ((v - min) / span) * innerH;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');
}
```

Add the following keys to **both** language maps in the single file `web-app/src/lib/translations.ts` (`Language = 'en' | 'my'` only; no other language files exist).

English block (`translations.en`):

```ts
'admin_finance_card_title': 'Finance',
'admin_finance_card_subtitle': 'Income, refunds, and cash',
'finance_title': 'Finance',
'finance_net_income': 'Net Income',
'finance_gross': 'Gross Revenue',
'finance_refunds': 'Refunds',
'finance_cash': 'Cash Collected',
'finance_in_progress': 'In progress',
'finance_export_csv': 'Export CSV',
'finance_empty': 'No paid activity in this range.',
'finance_wallet_topups': 'Wallet top-ups',
'finance_wallet_spend': 'Wallet spend',
'finance_orders': 'Successful orders',
'finance_customers': 'Unique customers',
'finance_aov': 'Average order value',
'finance_new_subs': 'New subscriptions',
'finance_extensions': 'Extensions',
'finance_admin_required': 'Admin access required to view finance.',
```

Myanmar block (`translations.my`):

```ts
'admin_finance_card_title': 'ဘဏ္ဍာရေး',
'admin_finance_card_subtitle': 'ဝင်ငွေ၊ ပြန်အမ်းငွေနှင့် ငွေသား',
'finance_title': 'ဘဏ္ဍာရေး',
'finance_net_income': 'အသားတင် ဝင်ငွေ',
'finance_gross': 'စုစုပေါင်း ဝင်ငွေ',
'finance_refunds': 'ပြန်အမ်းငွေ',
'finance_cash': 'ကောက်ခံငွေ',
'finance_in_progress': 'ဆောင်ရွက်ဆဲ',
'finance_export_csv': 'CSV ထုတ်ယူရန်',
'finance_empty': 'ဤကာလအတွင်း ငွေပေးချေမှု မရှိပါ။',
'finance_wallet_topups': 'Wallet ငွေဖြည့်',
'finance_wallet_spend': 'Wallet သုံးစွဲ',
'finance_orders': 'အောင်မြင်သော အော်ဒါများ',
'finance_customers': 'ထူးခြား ဖောက်သည်များ',
'finance_aov': 'ပျမ်းမျှ အော်ဒါတန်ဖိုး',
'finance_new_subs': 'စာရင်းသစ်များ',
'finance_extensions': 'သက်တမ်းတိုးများ',
'finance_admin_required': 'ဘဏ္ဍာရေးကို ကြည့်ရန် admin access လိုအပ်ပါသည်။',
```

- [ ] **Step 4: Run tests pass**

Run: `cd web-app && npm test -- src/lib/finance.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web-app/src/lib/finance.ts web-app/src/lib/finance.test.ts web-app/src/lib/translations.ts
git commit -m "feat(web): finance types and display helpers"
```

---

### Task 12: AdminFinance page with pure SVG/CSS trend chart

**Files:**
- Create: `web-app/src/pages/AdminFinance.tsx`
- Create: `web-app/src/pages/AdminFinance.test.tsx`

**Consumes:** `/api/me`, `/api/revenue`, `/api/revenue/export`, finance helpers.  
**Produces:** responsive `/admin/finance` UI. **Do not add chart dependencies to `package.json`.**

- [ ] **Step 1: Write failing page tests**

```tsx
// web-app/src/pages/AdminFinance.test.tsx
import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminFinance } from './AdminFinance';
import { jsonResponse, renderWithAppProviders, seedTelegramSession } from '../test/test-utils';
import type { FinanceMetrics, FinanceReport } from '../lib/finance';

const telegramState = vi.hoisted(() => ({
  tg: {
    BackButton: {
      show: vi.fn(),
      hide: vi.fn(),
      onClick: vi.fn(),
      offClick: vi.fn(),
    },
    openLink: vi.fn(),
    initDataUnsafe: { user: { id: 42 } },
  },
  initData: 'test-init-data',
  user: { id: 42 },
  close: vi.fn(),
  openLink: vi.fn(),
  colorScheme: 'light',
  themeParams: {},
}));

vi.mock('../lib/twa', () => ({
  useTelegram: () => telegramState,
}));

vi.mock('../lib/useMXBrownSound', () => ({
  useMXBrownSound: () => ({ playClick: vi.fn() }),
}));

const baseMetrics: FinanceMetrics = {
  gross_service_revenue: 1000,
  refunds: 100,
  net_service_revenue: 900,
  cash_collected: 800,
  wallet_topups: 200,
  wallet_spend: 200,
  successful_orders: 2,
  unique_customers: 2,
  average_order_value: 500,
  new_subscriptions: 1,
  extensions: 1,
};

const sampleCategories = [{ category: 'new_key', orders: 1, amount: 600 }];
const sampleMethods = [{
  method: 'kbz',
  transactions: 1,
  service_revenue: 600,
  cash_collected: 600,
  wallet_topups: 0,
  wallet_spend: 0,
}];

const sampleReport: FinanceReport = {
  period: 'day',
  timezone: 'Asia/Yangon',
  currency: 'MMK',
  range_start: '2026-07-12',
  range_end: '2026-07-12',
  generated_at: '2026-07-12T10:00:00+06:30',
  in_progress: true,
  current: baseMetrics,
  prior: null,
  delta: null,
  categories: sampleCategories,
  methods: sampleMethods,
  trend: [{
    period_start: '2026-07-12',
    period_end: '2026-07-12',
    in_progress: true,
    metrics: baseMetrics,
    categories: sampleCategories,
    methods: sampleMethods,
  }],
};

describe('AdminFinance', () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal('fetch', fetchMock);
    seedTelegramSession();
  });

  it('blocks non-admin users', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/me') {
        return jsonResponse({
          user: { id: 1, telegram_id: 42 },
          keys: [],
          is_active: false,
          expire_at: null,
          days_remaining: 0,
          trial_eligible: false,
          trial_days: 0,
          is_admin: false,
        });
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    renderWithAppProviders([
      { path: '/admin/finance', element: <AdminFinance /> },
      { path: '/', element: <div>Home</div> },
    ], ['/admin/finance']);

    expect(await screen.findByRole('alert')).toHaveTextContent(/Admin access required/i);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('shows net income, in progress, and svg chart for admins', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/me') {
        return jsonResponse({
          user: { id: 1, telegram_id: 42 },
          keys: [],
          is_active: false,
          expire_at: null,
          days_remaining: 0,
          trial_eligible: false,
          trial_days: 0,
          is_admin: true,
        });
      }
      if (url.startsWith('/api/revenue?')) {
        return jsonResponse(sampleReport);
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    renderWithAppProviders([
      { path: '/admin/finance', element: <AdminFinance /> },
    ], ['/admin/finance']);

    expect(await screen.findByRole('heading', { name: /Finance/i })).toBeTruthy();
    expect(screen.getByText(/900\.00/)).toBeTruthy();
    expect(screen.getByText(/In progress/i)).toBeTruthy();
    expect(screen.getByRole('img', { name: /Finance trend/i })).toBeTruthy();
  });

  it('refetches when weekly tab is selected', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/me') {
        return jsonResponse({
          user: { id: 1, telegram_id: 42 },
          keys: [],
          is_active: false,
          expire_at: null,
          days_remaining: 0,
          trial_eligible: false,
          trial_days: 0,
          is_admin: true,
        });
      }
      if (url.startsWith('/api/revenue?')) {
        return jsonResponse({ ...sampleReport, period: url.includes('week') ? 'week' : 'day' });
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    renderWithAppProviders([
      { path: '/admin/finance', element: <AdminFinance /> },
    ], ['/admin/finance']);

    await screen.findByRole('heading', { name: /Finance/i });
    fireEvent.click(screen.getByRole('button', { name: /Weekly/i }));
    await waitFor(() => {
      const revenueCalls = fetchMock.mock.calls
        .map((c) => String(c[0]))
        .filter((u) => u.includes('/api/revenue?'));
      expect(revenueCalls.some((u) => u.includes('period=week'))).toBe(true);
    });
  });

  it('shows session expired on 401', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/me') {
        return jsonResponse({
          user: { id: 1, telegram_id: 42 },
          keys: [],
          is_active: false,
          expire_at: null,
          days_remaining: 0,
          trial_eligible: false,
          trial_days: 0,
          is_admin: true,
        });
      }
      if (url.startsWith('/api/revenue?')) {
        return jsonResponse('unauthorized', 401);
      }
      if (url === '/api/session') {
        return jsonResponse({ token: 'renewed-session-token' });
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    renderWithAppProviders([
      { path: '/admin/finance', element: <AdminFinance /> },
    ], ['/admin/finance']);

    expect(await screen.findByRole('heading', { name: /Session expired/i })).toBeTruthy();
  });

  it('requests CSV export URL', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url === '/api/me') {
        return jsonResponse({
          user: { id: 1, telegram_id: 42 },
          keys: [],
          is_active: false,
          expire_at: null,
          days_remaining: 0,
          trial_eligible: false,
          trial_days: 0,
          is_admin: true,
        });
      }
      if (url.startsWith('/api/revenue?') && !url.includes('export')) {
        return jsonResponse(sampleReport);
      }
      if (url.startsWith('/api/revenue/export?')) {
        return new Response('section,key,value\ncurrent,net_service_revenue,900.00\n', {
          status: 200,
          headers: { 'Content-Type': 'text/csv' },
        });
      }
      throw new Error(`Unhandled fetch: ${url}`);
    });

    renderWithAppProviders([
      { path: '/admin/finance', element: <AdminFinance /> },
    ], ['/admin/finance']);

    await screen.findByRole('heading', { name: /Finance/i });
    fireEvent.click(screen.getByRole('button', { name: /Export CSV/i }));
    await waitFor(() => {
      const exportCalls = fetchMock.mock.calls
        .map((c) => String(c[0]))
        .filter((u) => u.includes('/api/revenue/export?'));
      expect(exportCalls.length).toBeGreaterThan(0);
    });
  });
});
```

- [ ] **Step 2: Run expecting fail**

Run: `cd web-app && npm test -- src/pages/AdminFinance.test.tsx`
Expected: FAIL cannot find module `./AdminFinance`.

- [ ] **Step 3: Implement AdminFinance.tsx**

```tsx
// web-app/src/pages/AdminFinance.tsx — core structure
import { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import {
  fetchJSONWithTelegramAuth,
  fetchUserScopedJSONWithTelegramAuth,
  fetchWithTelegramAuth,
} from '../lib/auth';
import { APIError, isAPIStatus } from '../lib/http';
import {
  buildRevenueExportQuery,
  buildRevenueQuery,
  buildTrendPolylinePoints,
  formatDelta,
  formatMoneyMMK,
  type FinancePeriod,
  type FinanceReport,
} from '../lib/finance';
import { useLanguage } from '../lib/LanguageContext';
import { UserData } from '../lib/types';
import { useTelegram } from '../lib/twa';
import { useMXBrownSound } from '../lib/useMXBrownSound';

function FinanceTrendChart({ report }: { report: FinanceReport }) {
  const width = 320;
  const height = 140;
  const pad = 16;
  const gross = report.trend.map((b) => b.metrics.gross_service_revenue);
  const refunds = report.trend.map((b) => b.metrics.refunds);
  const net = report.trend.map((b) => b.metrics.net_service_revenue);
  const grossPts = buildTrendPolylinePoints(gross, width, height, pad);
  const refundPts = buildTrendPolylinePoints(refunds, width, height, pad);
  const netPts = buildTrendPolylinePoints(net, width, height, pad);
  return (
    <svg viewBox={`0 0 ${width} ${height}`} width="100%" role="img" aria-label="Finance trend">
      <polyline fill="none" stroke="var(--digital-card-hint)" strokeWidth="1.5" points={grossPts} />
      <polyline fill="none" stroke="#e74c3c" strokeWidth="1.5" points={refundPts} />
      <polyline fill="none" stroke="var(--digital-card-text)" strokeWidth="2" points={netPts} />
    </svg>
  );
}

export function AdminFinance() {
  const { t } = useLanguage();
  const { playClick } = useMXBrownSound();
  const navigate = useNavigate();
  const { tg, initData } = useTelegram();
  const [period, setPeriod] = useState<FinancePeriod>('day');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [report, setReport] = useState<FinanceReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [isAdmin, setIsAdmin] = useState<boolean | null>(null);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const load = useCallback(async () => {
    if (!initData) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const me = await fetchUserScopedJSONWithTelegramAuth<UserData>(
        '/api/me',
        initData,
        tg?.initDataUnsafe?.user?.id,
      );
      if (!me.is_admin) {
        setIsAdmin(false);
        setLoading(false);
        return;
      }
      setIsAdmin(true);
      const url =
        period === 'custom'
          ? buildRevenueQuery({ period, from, to })
          : buildRevenueQuery({ period, periods: period === 'day' ? 30 : period === 'year' ? 5 : 12 });
      const data = await fetchJSONWithTelegramAuth<FinanceReport>(url, initData);
      setReport(data);
    } catch (e) {
      if (isAPIStatus(e, 401)) {
        setSessionExpired(true);
      } else {
        setError(e instanceof APIError ? e.body || e.message : 'Failed to load finance');
      }
    } finally {
      setLoading(false);
    }
  }, [period, from, to, initData, tg]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    tg?.BackButton?.show();
    const handler = () => navigate('/');
    tg?.BackButton?.onClick(handler);
    return () => {
      tg?.BackButton?.offClick(handler);
      tg?.BackButton?.hide();
    };
  }, [navigate, tg]);

  const onExport = async () => {
    if (!initData) return;
    playClick();
    const url =
      period === 'custom'
        ? buildRevenueExportQuery({ period, from, to })
        : buildRevenueExportQuery({ period, periods: period === 'day' ? 30 : period === 'year' ? 5 : 12 });
    const res = await fetchWithTelegramAuth(url, initData);
    if (!res.ok) {
      setError('Export failed');
      return;
    }
    const blob = await res.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'finance-report.csv';
    a.click();
    URL.revokeObjectURL(a.href);
  };

  if (sessionExpired) {
    return (
      <SessionExpiredScreen
        title={t('session_expired_title')}
        message={t('session_expired_desc')}
        reloadLabel={t('session_expired_reload')}
        closeLabel={t('session_expired_close')}
      />
    );
  }
  if (loading || isAdmin === null) return <LoadingScreen />;
  if (isAdmin === false) {
    return <div role="alert">{t('finance_admin_required')}</div>;
  }
  if (error) {
    return (
      <div role="alert">
        {error}
        <button type="button" onClick={() => void load()}>Retry</button>
      </div>
    );
  }

  return (
    <div style={{ padding: 16, maxWidth: 480, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
        <h1 style={{ margin: 0 }}>{t('finance_title')}</h1>
        <button type="button" onClick={() => void onExport()}>{t('finance_export_csv')}</button>
      </div>
      <div style={{ fontSize: 12, color: 'var(--digital-card-hint)' }}>
        {report?.timezone ?? 'Asia/Yangon'} · {report?.range_start}
        {report?.in_progress ? ` · ${t('finance_in_progress')}` : ''}
      </div>

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 12 }}>
        {([
          ['day', 'Daily'],
          ['week', 'Weekly'],
          ['month', 'Monthly'],
          ['year', 'Yearly'],
          ['custom', 'Custom'],
        ] as const).map(([value, label]) => (
          <button
            key={value}
            type="button"
            aria-pressed={period === value}
            onClick={() => {
              playClick();
              setPeriod(value);
            }}
          >
            {label}
          </button>
        ))}
      </div>

      {period === 'custom' && (
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
          <input aria-label="From date" type="date" value={from} onChange={(e) => setFrom(e.target.value)} />
          <input aria-label="To date" type="date" value={to} onChange={(e) => setTo(e.target.value)} />
        </div>
      )}

      {report && (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, marginTop: 16 }}>
            <div className="digital-card" style={{ padding: 12, gridColumn: '1 / -1' }}>
              <div>{t('finance_net_income')}</div>
              <strong>{formatMoneyMMK(report.current.net_service_revenue)}</strong>
              {report.in_progress && <span> · {t('finance_in_progress')}</span>}
              {report.delta && <div>{formatDelta(report.delta.net_service_revenue)}</div>}
            </div>
            <div className="digital-card" style={{ padding: 12 }}>
              <div>{t('finance_gross')}</div>
              <strong>{formatMoneyMMK(report.current.gross_service_revenue)}</strong>
            </div>
            <div className="digital-card" style={{ padding: 12 }}>
              <div>{t('finance_refunds')}</div>
              <strong>{formatMoneyMMK(report.current.refunds)}</strong>
            </div>
            <div className="digital-card" style={{ padding: 12 }}>
              <div>{t('finance_cash')}</div>
              <strong>{formatMoneyMMK(report.current.cash_collected)}</strong>
            </div>
          </div>

          <div style={{ marginTop: 16 }}>
            <FinanceTrendChart report={report} />
          </div>

          <ul style={{ marginTop: 16, paddingLeft: 18 }}>
            <li>{t('finance_wallet_topups')}: {formatMoneyMMK(report.current.wallet_topups)}</li>
            <li>{t('finance_wallet_spend')}: {formatMoneyMMK(report.current.wallet_spend)}</li>
            <li>{t('finance_new_subs')}: {report.current.new_subscriptions}</li>
            <li>{t('finance_extensions')}: {report.current.extensions}</li>
            <li>{t('finance_orders')}: {report.current.successful_orders}</li>
            <li>{t('finance_customers')}: {report.current.unique_customers}</li>
            <li>{t('finance_aov')}: {formatMoneyMMK(report.current.average_order_value)}</li>
          </ul>

          <table style={{ width: '100%', marginTop: 16, fontSize: 13 }}>
            <thead>
              <tr>
                <th align="left">Period</th>
                <th align="right">Net</th>
                <th align="right">Gross</th>
                <th align="right">Refunds</th>
              </tr>
            </thead>
            <tbody>
              {report.trend.map((b) => (
                <tr key={b.period_start}>
                  <td>
                    <button
                      type="button"
                      onClick={() =>
                        setExpanded((prev) => ({ ...prev, [b.period_start]: !prev[b.period_start] }))
                      }
                    >
                      {b.period_start}{b.in_progress ? ' *' : ''}
                    </button>
                    {expanded[b.period_start] && (
                      <div>
                        {b.categories.map((c) => (
                          <div key={c.category}>{c.category}: {c.orders} / {formatMoneyMMK(c.amount)}</div>
                        ))}
                        {b.methods.map((m) => (
                          <div key={m.method}>{m.method}: {formatMoneyMMK(m.service_revenue)}</div>
                        ))}
                      </div>
                    )}
                  </td>
                  <td align="right">{formatMoneyMMK(b.metrics.net_service_revenue)}</td>
                  <td align="right">{formatMoneyMMK(b.metrics.gross_service_revenue)}</td>
                  <td align="right">{formatMoneyMMK(b.metrics.refunds)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      {report && report.current.successful_orders === 0 && report.current.gross_service_revenue === 0 && (
        <p>{t('finance_empty')}</p>
      )}
    </div>
  );
}
```

- [ ] **Step 4: Run tests pass**

Run: `cd web-app && npm test -- src/pages/AdminFinance.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web-app/src/pages/AdminFinance.tsx web-app/src/pages/AdminFinance.test.tsx
git commit -m "feat(web): AdminFinance page with SVG trend chart"
```

---

### Task 13: Route + Home navigation

**Files:**
- Modify: `web-app/src/App.tsx`
- Modify: `web-app/src/pages/Home.tsx`
- Modify: `web-app/src/pages/Home.test.tsx`

**Consumes:** `AdminFinance`, translation keys.  
**Produces:** `/admin/finance` route + Home Finance card for `is_admin`.

- [ ] **Step 1: Write failing Home tests (extend existing file patterns)**

```tsx
// append inside describe('Home') in web-app/src/pages/Home.test.tsx

it('shows Finance admin card for admins', async () => {
  fetchMock.mockResolvedValueOnce(jsonResponse({
    user: { id: 1, telegram_id: 42 },
    keys: [],
    is_active: false,
    expire_at: null,
    days_remaining: 0,
    trial_eligible: false,
    trial_days: 0,
    is_admin: true,
  }));

  renderWithAppProviders([
    { path: '/', element: <Home /> },
    { path: '/wallet', element: <div>Wallet</div> },
    { path: '/admin/plans', element: <div>Plan Admin</div> },
    { path: '/admin/promos', element: <div>Promo Admin</div> },
    { path: '/admin/finance', element: <div>Finance Admin</div> },
  ], ['/']);

  expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
  expect(screen.getByRole('link', { name: /Finance/i })).toBeTruthy();
});

it('hides Finance admin card for non-admin users', async () => {
  fetchMock.mockResolvedValueOnce(jsonResponse({
    user: { id: 1, telegram_id: 42 },
    keys: [],
    is_active: false,
    expire_at: null,
    days_remaining: 0,
    trial_eligible: false,
    trial_days: 0,
    is_admin: false,
  }));

  renderWithAppProviders([
    { path: '/', element: <Home /> },
    { path: '/wallet', element: <div>Wallet</div> },
    { path: '/admin/plans', element: <div>Plan Admin</div> },
    { path: '/admin/promos', element: <div>Promo Admin</div> },
    { path: '/admin/finance', element: <div>Finance Admin</div> },
  ], ['/']);

  expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
  expect(screen.queryByRole('link', { name: /Finance/i })).toBeNull();
});
```

Also update the existing admin promo card tests to assert Finance visibility consistently:

```tsx
// in 'shows an admin promo card only for admins' after promo assertions:
expect(screen.getByRole('link', { name: /Finance/i })).toBeTruthy();

// in 'hides the admin promo card for non-admin users':
expect(screen.queryByRole('link', { name: /Finance/i })).toBeNull();
```

- [ ] **Step 2: Run expecting fail**

Run: `cd web-app && npm test -- src/pages/Home.test.tsx`
Expected: FAIL missing Finance link.

- [ ] **Step 3: Wire route and card**

```tsx
// web-app/src/App.tsx
import { AdminFinance } from './pages/AdminFinance';
// inside Routes:
<Route path="/admin/finance" element={<AdminFinance />} />
```

```tsx
// web-app/src/pages/Home.tsx — place with other admin cards (before plans card)
{data?.is_admin && (
  <Link
    to="/admin/finance"
    className="digital-card animate-slide-up"
    style={{
      padding: '16px 20px', display: 'flex', alignItems: 'center', gap: 14,
      textDecoration: 'none', color: 'var(--digital-card-text)',
      transition: 'transform 0.15s cubic-bezier(0.34, 1.56, 0.64, 1)',
      cursor: 'pointer',
    }}
    onClick={() => playClick()}
    onMouseDown={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
    onMouseUp={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
    onTouchStart={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
    onTouchEnd={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
    onMouseLeave={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
  >
    <div style={{
      width: 44, height: 44, borderRadius: 12,
      background: 'var(--digital-card-inner-bg)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      color: 'var(--digital-card-text)',
    }} aria-hidden="true">
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M4 19V5" />
        <path d="M4 19h16" />
        <path d="M8 15l3-4 3 2 4-6" />
      </svg>
    </div>
    <div style={{ flex: 1 }}>
      <div style={{ fontWeight: 'var(--weight-bold)', fontSize: '15px' }}>
        {t('admin_finance_card_title')}
      </div>
      <div style={{ fontSize: '13px', color: 'var(--digital-card-hint)', marginTop: 1 }}>
        {t('admin_finance_card_subtitle')}
      </div>
    </div>
    <div style={{
      width: 28, height: 28, borderRadius: 14,
      background: 'var(--digital-card-inner-bg)',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      fontSize: 14,
    }} aria-hidden="true">→</div>
  </Link>
)}
```

- [ ] **Step 4: Run tests pass**

Run: `cd web-app && npm test -- src/pages/Home.test.tsx src/pages/AdminFinance.test.tsx`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web-app/src/App.tsx web-app/src/pages/Home.tsx web-app/src/pages/Home.test.tsx
git commit -m "feat(web): register /admin/finance and Home navigation card"
```

---

### Task 14: Documentation

**Files:**
- Modify: `docs/MINI_APP.md`
- Modify: `HOWTOUSE.md`

**Consumes:** final API + UI behavior.  
**Produces:** operator-facing docs.

- [ ] **Step 1: Update Mini App docs**

Append to `docs/MINI_APP.md`:

```markdown
## Admin Finance

- Route: `/admin/finance` (admin session required).
- Data source: `GET /api/revenue` returns a structured `FinanceReport` (not raw purchase rows).
- CSV: `GET /api/revenue/export` with the same query params; totals match JSON.
- Timezone: all period boundaries are `Asia/Yangon`.
- Metrics:
  - Gross service revenue: paid plan purchases (includes wallet spend; excludes wallet top-ups)
  - Refunds: `financial_adjustment` rows with `adjustment_type=refund` on effective date
  - Net Income: gross − refunds
  - Cash collected: external money including wallet top-ups
- The browser never aggregates money; it only renders server values.
- Trend chart is pure SVG (no chart library).
```

- [ ] **Step 2: Update HOWTOUSE ops docs**

Append to `HOWTOUSE.md`:

```markdown
## Structured finance reporting

1. Open the Mini App as admin → **Finance** card → `/admin/finance`.
2. Use Daily/Weekly/Monthly/Yearly tabs or a custom Yangon date range.
3. Export CSV from the page (same totals as on-screen JSON metrics).
4. Telegram `/revenue` and scheduled daily/weekly/monthly jobs use the same `FinanceService` definitions (Net Income, Gross, Refunds, Cash).

### Recording a service refund

Service refunds are **not** inferred from purchase status and are **not** wallet cleanup refunds.

```http
POST /api/admin/financial-adjustments
Authorization: <admin mini-app session>
Content-Type: application/json

{
  "adjustment_type": "refund",
  "amount": 1000.00,
  "currency": "MMK",
  "purchase_id": 123,
  "effective_at": "2026-07-12T10:00:00+06:30",
  "reason": "customer request",
  "external_ref": "bank-txn-1",
  "idempotency_key": "refund:123:bank-txn-1"
}
```

- Replay with the same `idempotency_key` returns the existing row (HTTP 200).
- This endpoint writes only the finance ledger. It does **not** change purchase fulfillment, Remnawave state, or wallet balances.
- Wallet cleanup refunds (`wallet_transaction.type = refund`) are operational wallet corrections and must not be entered as service refunds.
- Historical refunds are not auto-backfilled; enter explicit adjustments after reconciliation.
```

- [ ] **Step 3: Commit**

```bash
git add docs/MINI_APP.md HOWTOUSE.md
git commit -m "docs: structured financial reporting and refund adjustment ops"
```

---

### Task 15: Full verification gate

**Files:** none intentionally (fix only if verification finds regressions).

- [ ] **Step 1: Backend verification**

```bash
go test ./...
go vet ./...
go build ./cmd/app
```

Expected: all PASS / clean vet / build success.

- [ ] **Step 2: Frontend verification**

```bash
cd web-app && npm ci && npm test && npm run build
```

Expected: vitest PASS; `tsc && vite build` PASS; `package.json` still has no chart library (`react`, `react-dom`, `react-router-dom` only in dependencies).

- [ ] **Step 3: Manual checklist**

1. Migration `000033` applies cleanly.
2. Create refund adjustment twice with same idempotency key → one row.
3. Paid sale day D1, refund effective D2 → gross on D1, refunds on D2, nets correct.
4. Wallet top-up increases cash + wallet_topups, not gross service revenue.
5. Wallet spend increases gross + wallet_spend, not cash.
6. Admin telegram purchases excluded from all metrics + CSV.
7. JSON net == CSV net for same query.
8. Invalid period/range → 400; DB failure → 500.
9. `/admin/finance` loads in narrow viewport; SVG chart visible; export downloads.
10. Telegram `/revenue` shows Net Income line.
11. Confirm `internal/payment` fulfillment/wallet mutation files unchanged.

- [ ] **Step 4: Final commit only if verification fixes were required**

```bash
git add -A
git commit -m "fix: financial reporting verification follow-ups"
```

---

## Self-Review

### 1. Spec coverage

| Design requirement | Task(s) |
|--------------------|---------|
| 000033 financial adjustment ledger + idempotency | 1, 2 |
| Admin-only adjustment create API (ledger-only, no fulfillment coupling) | 8 |
| Year + custom ranges, Yangon boundaries, partial in-progress | 3, 5, 7, 9 |
| Gross / refunds / net / cash / wallet / orders / customers / AOV / category / method | 5, 9, 12 |
| Prior comparison + named `FinanceDelta` + trend DTO | 5 |
| Zero-safe totals via authoritative `Period*` fields + unique customers across range | 4, 7 |
| Admin exclusions everywhere | 2, existing purchase SQL, 7 count, 9 export |
| JSON + CSV same DTO | 6, 9 |
| API auth/validation/bounds + 400 vs 500 split | 8, 9 |
| Telegram + cron shared `FinanceService` | 7, 10 |
| `/admin/finance` UI + pure SVG chart (no chart dep) | 12 |
| Home nav + route | 13 |
| Frontend states/tests | 11–13 |
| Docs | 14 |
| Full verification | 15 |
| No payment fulfillment / wallet mutation changes | Architecture lock + Task 7 service outside payment |
| Wallet cleanup refunds ≠ service refunds | Constraints + Tasks 2, 5, 8, 14 |
| float64 money; 2-decimal boundaries; integer conversion out of scope | Header + Tasks 2, 4, 6 |

### 2. Placeholder / ambiguity scan (literal)

**Literal scan terms checked this revision:**
`TBD`, `TODO`, `if practical`, `if needed`, `only if necessary`, `prefer `, `alternative assembly`, `chatIDFromUpdate`, `both languages`, `use existing`, `preserve`, `authHeaders`, `Workers must`, `copy exact`, `_ = cs`, `_ = ps`, `report.categories.map` (must not appear in expandable row UI), anonymous delta struct, PaymentService assembly branches, empty test bodies, comment-only implementation steps.

**Findings repaired in this pass:**
1. Clock: documented existing `now` / `currentTime()` wiring; `NewAPIHandler` sets `now: time.Now`; finance handlers use `h.currentTime()`.
2. Telegram: full `RevenueCommandHandler` uses `update.Message.Chat.ID` only; removed `chatIDFromUpdate`.
3. Translations: exact keys/values for `en` and `my` in `web-app/src/lib/translations.ts` only.
4. Day window test: single real assertion block; no-op `_ = cs` removed.
5. Expandable rows: `FinanceTrendBucket.Categories`/`Methods` period-specific; builder, fixtures, TS types, UI render `b.categories`/`b.methods`.

**Remaining non-placeholder operational notes:**
- Task 15: verification-only; commit only when fixes land.

**No unresolved architecture branches remain.** Assembly is exclusively `internal/reporting/service.go` `FinanceService`. Named `FinanceDelta` is used in Go and TS. API uses `telegramIDKey` + `financeReporter` / `financialAdjustmentCreator` injection with `now: time.Now` / `currentTime()`. Trend buckets carry period-specific `Categories`/`Methods`. Telegram uses `update.Message.Chat.ID`. Translations are exact `en` + `my` keys in `web-app/src/lib/translations.ts`. 400 vs 500 mapping is explicit.

### 3. Type consistency

- `FinanceDelta` is a named Go type and mirrored TS interface.
- `FinanceService.GetReport(ctx, ReportQuery) (FinanceReport, error)` is the only assembly entrypoint.
- API injects `financeReporter` + `financialAdjustmentCreator` via `NewAPIHandler` trailing params; routes registered in `RegisterHandlers`.
- Admin telegram id via `r.Context().Value(telegramIDKey).(int64)` with `created_by = fmt.Sprintf("admin:%d", telegramID)`.
- Task 7 Files/steps/commit staging list the same six paths only.

---

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-12-structured-financial-reporting.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks. **REQUIRED SUB-SKILL:** `superpowers:subagent-driven-development`
2. **Inline Execution** — execute in this session with `superpowers:executing-plans`

Which approach?
