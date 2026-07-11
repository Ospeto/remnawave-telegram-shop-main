# Structured Financial Reporting Design

**Date:** 2026-07-12  
**Status:** Approved

## Objective

Create a dedicated Mini App finance page at `/admin/finance` that reports daily, weekly, monthly, yearly, and custom-range income accurately. Preserve the distinction between earned service revenue and cash movement, make refunds auditable, and use one reporting implementation across the Mini App, CSV exports, Telegram reports, and scheduled summaries.

## Decisions

- Primary income metric: net service revenue.
- Show both the current in-progress period and historical periods.
- Include the full operational breakdown in the initial release.
- Recognize refunds as negative adjustments on the refund date.
- Support Yangon-local custom date ranges and CSV export.
- Extend the existing reporting layer rather than introduce a competing reporting system or precomputed aggregation tables.

## Accounting Model

- **Gross service revenue:** Paid plan purchases, including purchases funded by wallet balance. Wallet top-ups are excluded.
- **Refunds:** Negative financial adjustments recognized on their effective dates.
- **Net service revenue:** Gross service revenue minus refunds. The UI labels this primary metric as **Net Income**.
- **Cash collected:** External money received, including wallet top-ups.
- **Wallet top-ups:** Cash received and stored as a customer liability; not service revenue.
- **Wallet spend:** Service revenue paid from stored balance; not new cash collection.
- **Orders and customers:** Successful plan purchases only.

All reporting boundaries use `Asia/Yangon`:

- Day: calendar day.
- Week: Monday through Sunday.
- Month: calendar month.
- Year: calendar year.
- Custom range: inclusive local start and end dates.

The UI clearly marks a current, incomplete period as **In progress**.

## Architecture

The existing database revenue aggregation remains the authoritative reporting path. It will be extended to support yearly and custom-range reporting, refunds, comparisons, and detailed breakdowns. Browser code only renders server-calculated values and never performs financial aggregation.

The same application reporting service supplies:

- Admin JSON responses.
- CSV exports.
- Telegram revenue reports.
- Scheduled daily, weekly, and monthly summaries.

The implementation must also correct known inconsistencies in the existing path:

- Use explicit metric names instead of relying on ambiguous `total_revenue` semantics.
- Apply admin/test exclusions to every metric and export.
- Calculate unique customers across the requested range rather than summing per-bucket unique counts.
- Preserve legitimate zero values during summary construction.
- Use Yangon-local boundaries consistently.

## Refund Adjustment Ledger

Accurate refund-date accounting requires a durable event record instead of inference from a purchase's current status. Add a small financial-adjustment ledger with:

- Purchase ID.
- Adjustment type, initially `refund`.
- Monetary amount.
- Effective timestamp.
- Optional reason or external reference.
- Created timestamp.
- Administrator or system origin.
- Idempotency key or equivalent uniqueness guarantee.

The original paid sale remains in its historical period. A refund appears as a negative adjustment in the period when it occurs. No existing ambiguous refund records are automatically backfilled; they require explicit reconciliation.

## Admin Finance Interface

Add a Finance card to the admin section of the Mini App Home page and register `/admin/finance`.

### Header and controls

- Page title: **Finance**.
- Current Yangon date and timezone indicator.
- CSV export action.
- Custom date-range controls.
- Tabs for Daily, Weekly, Monthly, and Yearly views.
- Historical navigation while retaining the current period.

### Summary cards

1. Net Income, visually primary.
2. Gross Revenue.
3. Refunds.
4. Cash Collected.

Each card shows the selected period's amount and its absolute and percentage difference from the preceding equivalent period. Partial periods carry the **In progress** label.

### Operational breakdown

- Wallet top-ups.
- Wallet spend.
- New subscriptions.
- Extensions.
- Successful orders.
- Unique customers.
- Average order value.
- Payment-method totals.

### Visualization and history

- A trend chart appropriate to the selected period.
- Gross revenue, refunds, and net income shown separately.
- A structured period table beneath the chart.
- Expandable rows for category and payment-method details.
- Loading, empty, authorization-expired, and error states consistent with existing Mini App conventions.
- A responsive layout optimized for Telegram's narrow viewport.

## API Design

Extend the existing admin reporting endpoints:

- `GET /api/revenue?period=day|week|month|year`
- Optional bounded history/pagination parameters.
- Optional Yangon-local `from` and `to` dates for a custom range.
- `GET /api/revenue/export?...` for CSV using the same filters and calculations.

The JSON result includes:

- Range metadata and partial-period status.
- Gross revenue, refunds, and net revenue.
- Cash and wallet metrics.
- Order count, unique customers, and average order value.
- Category and payment-method breakdowns.
- Prior-period comparisons.
- Ordered trend buckets.

Invalid periods and date ranges return clear `400` responses. Unauthorized requests preserve existing session-expired behavior. Report history and export size are bounded.

Money remains in integer monetary units through database and service layers and is formatted only for presentation. JSON and CSV must derive from the same report result so their totals cannot diverge.

## Testing

### Backend

- Daily, Monday-based weekly, monthly, and yearly Yangon boundaries.
- Year transitions and leap years.
- Current partial periods.
- Inclusive custom ranges.
- Gross revenue minus refunds equals net income.
- Refund recognition on the effective date.
- Wallet top-up and wallet-spend double-count prevention.
- Admin/test exclusion for every metric.
- Unique-customer counts across multi-bucket ranges.
- Prior-period comparisons.
- CSV totals matching JSON totals.
- Invalid ranges and admin authorization.
- Database-backed aggregation tests where practical, rather than relying only on SQL-shape assertions.

### Frontend

- Finance route and Home navigation.
- Period switching and historical navigation.
- Summary values and comparison labels.
- Current-period **In progress** state.
- Custom date filtering.
- CSV export.
- Expanded breakdown rows.
- Loading, empty, API-error, and expired-session states.
- Narrow Telegram viewport behavior.

## Rollout

1. Add the financial-adjustment ledger and reporting changes.
2. Do not infer or automatically backfill historical refunds.
3. Reconcile representative API and CSV totals against purchases and wallet transactions.
4. Add `/admin/finance` and its Home navigation entry.
5. Route existing Telegram reports through the corrected shared reporting service.
6. Update Mini App and operations documentation.

Payment fulfillment and wallet-balance mutation logic are outside this change. Existing idempotency, transaction-ID uniqueness, and wallet-balance invariants must remain intact.
