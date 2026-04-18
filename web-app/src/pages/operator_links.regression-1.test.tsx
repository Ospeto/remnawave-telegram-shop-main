import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Home } from './Home';
import { Wallet } from './Wallet';
import { Checkout } from './Checkout';
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

vi.mock('../lib/twa', async () => {
    const actual = await vi.importActual<typeof import('../lib/twa')>('../lib/twa');
    return {
        ...actual,
        useTelegram: () => telegramState,
    };
});

vi.mock('../lib/useMXBrownSound', () => ({
    useMXBrownSound: () => ({ playClick: vi.fn() }),
}));

describe('operator link regressions', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        telegramState.tg.openTelegramLink.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('uses the configured support URL on Home and falls back to the shop bot URL', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            support_url: 'https://t.me/custom-support',
            bot_url: 'https://t.me/MyShopBot',
        }));

        const { unmount } = renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/']);

        expect(await screen.findByRole('link', { name: /Contact Support/i })).toHaveAttribute('href', 'https://t.me/custom-support');

        unmount();
        fetchMock.mockReset();
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            bot_url: 'https://t.me/MyShopBot',
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/']);

        expect(await screen.findByRole('link', { name: /Contact Support/i })).toHaveAttribute('href', 'https://t.me/MyShopBot');
    });

    it('hides the wallet referral share button when the shop bot URL is missing', async () => {
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({
                    balance: 5000,
                    currency: 'MMK',
                    auto_renew: false,
                    auto_renew_duration: null,
                    referral_count: 1,
                    referral_earned: 1000,
                    referral_bonus_amount: 1000,
                });
            }
            if (url === '/api/wallet/history?limit=10') {
                return jsonResponse([]);
            }
            if (url === '/api/referrals') {
                return jsonResponse([]);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/wallet', element: <Wallet /> },
            { path: '/', element: <div>Home</div> },
        ], ['/wallet']);

        expect(await screen.findByRole('heading', { name: 'Wavy Wallet' })).toBeTruthy();
        expect(screen.queryByRole('button', { name: /Share your link/i })).toBeNull();
        expect(telegramState.tg.openTelegramLink).not.toHaveBeenCalled();
    });

    it('hides the checkout referral share button when purchase bot_url is missing', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'starter-plan', label: 'Starter', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
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
                return jsonResponse({ balance: 10000, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                return jsonResponse({
                    purchase_id: 99,
                    amount: 5000,
                    currency: 'MMK',
                    invoice_type: 'wallet',
                    happ_link: 'happ://import',
                    redirect_url: 'https://example.com/import',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/starter-plan']);

        fireEvent.click(await screen.findByRole('button', { name: 'Pay 5,000 MMK from Wallet' }));

        await screen.findByRole('heading', { name: /Payment Verified!/i });
        await waitFor(() => {
            expect(screen.queryByRole('button', { name: /^Share link$/i })).toBeNull();
        });
    });
});
