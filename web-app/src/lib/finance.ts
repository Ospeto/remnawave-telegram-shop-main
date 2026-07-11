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
