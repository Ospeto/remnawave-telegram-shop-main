import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Checkout } from './Checkout';
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
        openTelegramLink: vi.fn(),
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

describe('Checkout', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
    });

    it('loads wallet top-up checkout from query state and creates a top-up purchase without plan_index', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
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
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                if (body.payment_method !== 'wallet_topup') {
                    throw new Error(`Unexpected purchase body: ${JSON.stringify(body)}`);
                }

                return jsonResponse({
                    purchase_id: 99,
                    payment_phone: '09123456789',
                    amount: 5000,
                    currency: 'MMK',
                    instructions: 'Pay now',
                    invoice_type: 'wallet_topup',
                    bot_url: 'https://t.me/WavyVpnBot',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout?walletTopup=true&amount=5000']);

        const createButton = await screen.findByRole('button', { name: 'Create payment request' });
        expect(screen.queryByText('Invalid plan selected')).toBeNull();

        fireEvent.click(createButton);

        await screen.findByText('How to pay');
        await waitFor(() => {
            const purchaseCall = fetchMock.mock.calls.find(([url]) => url === '/api/purchase');
            expect(purchaseCall).toBeTruthy();

            const [, options] = purchaseCall as [string, RequestInit];
            const body = JSON.parse(String(options.body));
            expect(body.payment_method).toBe('wallet_topup');
            expect(body.amount).toBe(5000);
            expect(body.plan_index).toBeUndefined();
        });
    });
});
