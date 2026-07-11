export type FinancePeriod = 'day' | 'week' | 'month' | 'year' | 'custom';

/** Fixed (non-custom) periods that may include a history `periods` count. */
export type FixedFinancePeriod = 'day' | 'week' | 'month' | 'year';

/**
 * Discriminated revenue query options.
 * - custom: both `from` and `to` required; `periods` not allowed
 * - day|week|month|year: optional `periods`; `from`/`to` not allowed
 */
export type RevenueQueryOptions =
  | {
      period: 'custom';
      from: string;
      to: string;
    }
  | {
      period: FixedFinancePeriod;
      periods?: number;
    };

/** Stable error message for malformed revenue query options (runtime). */
export const INVALID_REVENUE_QUERY = 'Invalid revenue query options';

/** Stable error message when trend series contains non-finite numbers. */
export const INVALID_TREND_VALUES = 'Invalid trend values';

/**
 * Stable error message when chart width/height/pad are non-finite,
 * non-positive, or leave a non-positive drawable area.
 */
export const INVALID_TREND_DIMENSIONS = 'Invalid trend chart dimensions';

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

const FIXED_PERIODS = new Set<string>(['day', 'week', 'month', 'year']);

/** Exact calendar date: YYYY-MM-DD with real month/day (rejects 2026-02-30). */
const DATE_YYYY_MM_DD = /^(\d{4})-(\d{2})-(\d{2})$/;

/**
 * Parse and validate a calendar date string.
 * Trims input; requires exact YYYY-MM-DD after trim; rejects impossible dates.
 * Returns the normalized (trimmed) date string.
 */
function parseCalendarDate(value: unknown): string {
  if (typeof value !== 'string') {
    throw new Error(INVALID_REVENUE_QUERY);
  }
  const trimmed = value.trim();
  const m = DATE_YYYY_MM_DD.exec(trimmed);
  if (!m) {
    throw new Error(INVALID_REVENUE_QUERY);
  }
  const year = Number(m[1]);
  const month = Number(m[2]);
  const day = Number(m[3]);
  // UTC construction avoids local TZ shifting the calendar day.
  const dt = new Date(Date.UTC(year, month - 1, day));
  if (
    dt.getUTCFullYear() !== year ||
    dt.getUTCMonth() !== month - 1 ||
    dt.getUTCDate() !== day
  ) {
    throw new Error(INVALID_REVENUE_QUERY);
  }
  return trimmed;
}

/**
 * Runtime validation for JS callers that bypass TypeScript.
 * Throws INVALID_REVENUE_QUERY for any malformed combination.
 * Returns a normalized options object (trimmed custom dates).
 */
function normalizeRevenueQueryOptions(opts: RevenueQueryOptions): RevenueQueryOptions {
  const raw = opts as {
    period?: unknown;
    from?: unknown;
    to?: unknown;
    periods?: unknown;
  };

  if (raw.period === 'custom') {
    if (raw.periods !== undefined) {
      throw new Error(INVALID_REVENUE_QUERY);
    }
    const from = parseCalendarDate(raw.from);
    const to = parseCalendarDate(raw.to);
    if (from > to) {
      throw new Error(INVALID_REVENUE_QUERY);
    }
    return { period: 'custom', from, to };
  }

  if (typeof raw.period === 'string' && FIXED_PERIODS.has(raw.period)) {
    if (raw.from !== undefined || raw.to !== undefined) {
      throw new Error(INVALID_REVENUE_QUERY);
    }
    if (raw.periods !== undefined) {
      if (
        typeof raw.periods !== 'number' ||
        !Number.isFinite(raw.periods) ||
        !Number.isInteger(raw.periods) ||
        raw.periods < 1
      ) {
        throw new Error(INVALID_REVENUE_QUERY);
      }
      return { period: raw.period as FixedFinancePeriod, periods: raw.periods };
    }
    return { period: raw.period as FixedFinancePeriod };
  }

  throw new Error(INVALID_REVENUE_QUERY);
}

/** Shared query-string construction for revenue JSON and CSV export endpoints. */
function buildRevenueSearchParams(opts: RevenueQueryOptions): URLSearchParams {
  const normalized = normalizeRevenueQueryOptions(opts);
  const q = new URLSearchParams();
  q.set('period', normalized.period);
  if (normalized.period === 'custom') {
    q.set('from', normalized.from);
    q.set('to', normalized.to);
  } else if (normalized.periods !== undefined) {
    q.set('periods', String(normalized.periods));
  }
  return q;
}

function buildRevenuePath(
  basePath: '/api/revenue' | '/api/revenue/export',
  opts: RevenueQueryOptions,
): string {
  return `${basePath}?${buildRevenueSearchParams(opts).toString()}`;
}

export function buildRevenueQuery(opts: RevenueQueryOptions): string {
  return buildRevenuePath('/api/revenue', opts);
}

export function buildRevenueExportQuery(opts: RevenueQueryOptions): string {
  return buildRevenuePath('/api/revenue/export', opts);
}

/**
 * Map values to SVG polyline points string for a pure SVG chart.
 *
 * Behavior (documented by tests):
 * - empty series → ''
 * - single value → one centered point
 * - non-finite values → throw INVALID_TREND_VALUES
 * - non-finite / non-positive dimensions, negative pad, or non-positive
 *   drawable area (pad*2 >= width or height) → throw INVALID_TREND_DIMENSIONS
 */
export function buildTrendPolylinePoints(
  values: number[],
  width: number,
  height: number,
  pad: number,
): string {
  if (
    !Number.isFinite(width) ||
    !Number.isFinite(height) ||
    !Number.isFinite(pad) ||
    width <= 0 ||
    height <= 0 ||
    pad < 0 ||
    pad * 2 >= width ||
    pad * 2 >= height
  ) {
    throw new Error(INVALID_TREND_DIMENSIONS);
  }
  if (values.length === 0) return '';
  if (values.some((v) => !Number.isFinite(v))) {
    throw new Error(INVALID_TREND_VALUES);
  }

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
