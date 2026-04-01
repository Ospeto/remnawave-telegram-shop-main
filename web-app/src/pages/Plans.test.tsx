import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Plans } from './Plans';
import { jsonResponse, renderWithAppProviders } from '../test/test-utils';

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

describe('Plans', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
    });

    it('builds wallet top-up checkout links without a negative plan index', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/plans') {
                return jsonResponse([
                    { label: '1 Month', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK' },
                ]);
            }
            if (url === '/api/me') {
                return jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [],
                    is_active: false,
                    expire_at: null,
                    days_remaining: 0,
                    trial_eligible: false,
                    trial_days: 0,
                });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/plans', element: <Plans /> },
            { path: '/checkout', element: <div>Checkout</div> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/plans?walletTopup=true']);

        const links = await screen.findAllByRole('link');
        const checkoutLinks = links
            .map((link) => link.getAttribute('href'))
            .filter((href): href is string => href !== null && href.startsWith('/checkout'));

        expect(checkoutLinks[0]).toBe('/checkout?walletTopup=true&amount=5000');
        expect(checkoutLinks.every((href) => !href.includes('/checkout/-1'))).toBe(true);
    });
});
