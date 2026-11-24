import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Home } from './Home';
import { Plans } from './Plans';
import { jsonResponse, renderWithAppProviders, seedTelegramSession } from '../test/test-utils';

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

const profile = (overrides: Record<string, unknown> = {}) => ({
    user: { id: 1, telegram_id: 42 },
    keys: [{
        id: 101,
        label: 'Active Key',
        username: 'user1',
        subscription_url: 'https://example.com/sub/active',
        status: 'active',
        days_remaining: 5,
        total_days: 30,
        traffic_used_gb: 1,
        traffic_limit_gb: 10,
        auto_renew: false,
    }],
    is_active: true,
    expire_at: null,
    days_remaining: 5,
    trial_eligible: false,
    trial_days: 0,
    ...overrides,
});

const planList = [
    { id: 'plan-30', label: '1 Month', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
];

const meCallCount = (fetchMock: ReturnType<typeof vi.fn>) =>
    fetchMock.mock.calls.filter(([url]) => String(url) === '/api/me').length;

describe('shared profile across route changes', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('does not refetch /api/me when walking Home -> Plans', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/me') return jsonResponse(profile());
            if (url === '/api/plans') return jsonResponse(planList);
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/plans', element: <Plans /> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(meCallCount(fetchMock)).toBe(1);

        fireEvent.click(screen.getByRole('link', { name: /Buy New Key/i }));

        expect(await screen.findByText('1 Month')).toBeTruthy();
        // The second screen reused the profile the first one already loaded.
        expect(meCallCount(fetchMock)).toBe(1);
        expect(fetchMock.mock.calls.filter(([url]) => String(url) === '/api/plans')).toHaveLength(1);
    });

    it('refetches /api/me after a mutation that changes the key list', async () => {
        let autoRenew = false;
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);
            if (url === '/api/me') {
                return jsonResponse(profile({
                    keys: [{
                        id: 101,
                        label: 'Active Key',
                        username: 'user1',
                        subscription_url: 'https://example.com/sub/active',
                        status: 'active',
                        days_remaining: 5,
                        total_days: 30,
                        traffic_used_gb: 1,
                        traffic_limit_gb: 10,
                        auto_renew: autoRenew,
                    }],
                }));
            }
            if (url === '/api/keys/autorenew') {
                autoRenew = JSON.parse(String(init?.body ?? '{}')).enabled === true;
                return jsonResponse({ ok: true });
            }
            if (url === '/api/plans') return jsonResponse(planList);
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/plans', element: <Plans /> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
        ], ['/']);

        expect(await screen.findByText('Active Key')).toBeTruthy();
        expect(meCallCount(fetchMock)).toBe(1);

        const autoRenewSwitch = await screen.findByRole('switch', { name: 'Auto-Renewal' });
        fireEvent.click(autoRenewSwitch);

        await waitFor(() => {
            expect(fetchMock.mock.calls.find(([url]) => String(url) === '/api/keys/autorenew')).toBeTruthy();
        });

        fireEvent.click(screen.getByRole('link', { name: /Buy New Key/i }));
        expect(await screen.findByText('1 Month')).toBeTruthy();

        // The mutation dropped the shared profile, so Plans reads it again.
        await waitFor(() => {
            expect(meCallCount(fetchMock)).toBe(2);
        });
    });
});
