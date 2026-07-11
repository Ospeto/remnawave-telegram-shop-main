import { describe, expect, it } from 'vitest';
import {
  formatMoneyMMK,
  formatDelta,
  buildRevenueQuery,
  buildRevenueExportQuery,
  buildTrendPolylinePoints,
  INVALID_REVENUE_QUERY,
  INVALID_TREND_VALUES,
  INVALID_TREND_DIMENSIONS,
  type RevenueQueryOptions,
} from './finance';

/** Bypass compile-time discrimination to exercise runtime validation. */
function asQuery(opts: unknown): RevenueQueryOptions {
  return opts as RevenueQueryOptions;
}

describe('finance helpers', () => {
  it('formats money with two decimals and grouping', () => {
    expect(formatMoneyMMK(900)).toBe('900.00');
    expect(formatMoneyMMK(1234567.5)).toBe('1,234,567.50');
  });

  describe('formatDelta', () => {
    it('formats positive absolute and percentage with leading +', () => {
      expect(formatDelta({ absolute: 100, percentage: 50 })).toBe('+100.00 (+50.0%)');
    });

    it('formats zero absolute and percentage without +', () => {
      expect(formatDelta({ absolute: 0, percentage: 0 })).toBe('0.00 (0.0%)');
    });

    it('formats negative absolute and percentage without +', () => {
      expect(formatDelta({ absolute: -100, percentage: -25.5 })).toBe('-100.00 (-25.5%)');
    });

    it('uses em dash when percentage is null', () => {
      expect(formatDelta({ absolute: -100, percentage: null })).toBe('-100.00 (—)');
    });

    it('uses em dash when percentage is NaN', () => {
      expect(formatDelta({ absolute: 100, percentage: Number.NaN })).toBe('+100.00 (—)');
    });
  });

  it('builds query string for custom range', () => {
    expect(buildRevenueQuery({ period: 'custom', from: '2026-01-01', to: '2026-01-31' }))
      .toBe('/api/revenue?period=custom&from=2026-01-01&to=2026-01-31');
  });

  it('builds export query', () => {
    expect(buildRevenueExportQuery({ period: 'week', periods: 8 }))
      .toBe('/api/revenue/export?period=week&periods=8');
  });

  it('builds fixed-period query without periods when omitted', () => {
    expect(buildRevenueQuery({ period: 'month' })).toBe('/api/revenue?period=month');
  });

  it('builds custom export query with shared path construction', () => {
    expect(
      buildRevenueExportQuery({ period: 'custom', from: '2026-01-01', to: '2026-01-31' }),
    ).toBe('/api/revenue/export?period=custom&from=2026-01-01&to=2026-01-31');
  });

  describe('revenue query runtime validation', () => {
    it('throws when custom is missing from', () => {
      expect(() => buildRevenueQuery(asQuery({ period: 'custom', to: '2026-01-31' }))).toThrow(
        INVALID_REVENUE_QUERY,
      );
    });

    it('throws when custom is missing to', () => {
      expect(() => buildRevenueQuery(asQuery({ period: 'custom', from: '2026-01-01' }))).toThrow(
        INVALID_REVENUE_QUERY,
      );
    });

    it('throws when custom includes periods', () => {
      expect(() =>
        buildRevenueQuery(
          asQuery({ period: 'custom', from: '2026-01-01', to: '2026-01-31', periods: 4 }),
        ),
      ).toThrow(INVALID_REVENUE_QUERY);
    });

    it('throws when non-custom includes from/to', () => {
      expect(() =>
        buildRevenueQuery(
          asQuery({ period: 'week', periods: 8, from: '2026-01-01', to: '2026-01-31' }),
        ),
      ).toThrow(INVALID_REVENUE_QUERY);
      expect(() =>
        buildRevenueExportQuery(asQuery({ period: 'day', from: '2026-01-01' })),
      ).toThrow(INVALID_REVENUE_QUERY);
    });

    it('throws for unknown period', () => {
      expect(() => buildRevenueQuery(asQuery({ period: 'quarter' }))).toThrow(
        INVALID_REVENUE_QUERY,
      );
    });
  });

  describe('buildTrendPolylinePoints', () => {
    it('builds svg polyline points for multi-value series', () => {
      const pts = buildTrendPolylinePoints([0, 100, 50], 300, 120, 10);
      expect(pts.split(' ').length).toBe(3);
      expect(pts).toMatch(/^\d+(\.\d+)?,\d+(\.\d+)? /);
      // Exact: pad=10, innerW=280, innerH=100; min=0 max=100 span=100
      // i=0 → 10.0,110.0; i=1 → 150.0,10.0; i=2 → 290.0,60.0
      expect(pts).toBe('10.0,110.0 150.0,10.0 290.0,60.0');
    });

    it('returns empty string for empty series', () => {
      expect(buildTrendPolylinePoints([], 300, 120, 10)).toBe('');
    });

    it('centers a single value on the drawable width', () => {
      // pad=10, innerW=280, innerH=100; value=50 with floor 0 → y at top of value range
      // x = 10 + 140 = 150; y = 10 + 100 - 100 = 10
      expect(buildTrendPolylinePoints([50], 300, 120, 10)).toBe('150.0,10.0');
    });

    it('rejects non-finite trend values', () => {
      expect(() => buildTrendPolylinePoints([0, Number.NaN, 2], 300, 120, 10)).toThrow(
        INVALID_TREND_VALUES,
      );
      expect(() => buildTrendPolylinePoints([Number.POSITIVE_INFINITY], 300, 120, 10)).toThrow(
        INVALID_TREND_VALUES,
      );
    });

    it('rejects invalid chart dimensions', () => {
      expect(() => buildTrendPolylinePoints([1, 2], Number.NaN, 120, 10)).toThrow(
        INVALID_TREND_DIMENSIONS,
      );
      expect(() => buildTrendPolylinePoints([1, 2], 300, 0, 10)).toThrow(INVALID_TREND_DIMENSIONS);
      expect(() => buildTrendPolylinePoints([1, 2], 300, 120, -1)).toThrow(
        INVALID_TREND_DIMENSIONS,
      );
      // pad*2 >= width leaves non-positive drawable area
      expect(() => buildTrendPolylinePoints([1, 2], 20, 120, 10)).toThrow(
        INVALID_TREND_DIMENSIONS,
      );
    });
  });
});
