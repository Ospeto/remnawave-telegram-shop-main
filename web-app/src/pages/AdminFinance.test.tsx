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
    // Net appears on the headline card and again in the trend table row.
    expect(screen.getAllByText(/900\.00/).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/In progress/i).length).toBeGreaterThan(0);
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
