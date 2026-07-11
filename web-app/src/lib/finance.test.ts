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
