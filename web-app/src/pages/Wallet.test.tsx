import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Wallet } from './Wallet';
import { jsonResponse, renderWithAppProviders, seedTelegramSession } from '../test/test-utils';
import { invalidateUserData, sharedUserDataStore } from '../lib/userData';

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

describe('Wallet', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('renders wallet data and shows a referral warning when referrals fail to load', async () => {
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
            if (url === '/api/wallet') {
                return jsonResponse({
                    balance: 5000,
                    currency: 'MMK',
                    auto_renew: false,
                    auto_renew_duration: null,
                    bot_url: 'https://t.me/WavyVpnBot',
                    referral_count: 2,
                    referral_earned: 2500,
                    referral_bonus_amount: 1000,
                });
            }
            if (url === '/api/wallet/history?limit=10') {
                return jsonResponse([]);
            }
            if (url === '/api/referrals') {
                return jsonResponse('boom', 500);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/wallet', element: <Wallet /> },
            { path: '/', element: <div>Home</div> },
        ], ['/wallet']);

        expect(await screen.findByRole('heading', { name: 'Wavy Wallet' })).toBeTruthy();
        expect(screen.getByRole('button', { name: '+ Top Up Balance' })).toBeTruthy();
        expect(screen.getByText('Referral activity is temporarily unavailable. Your totals are still shown above.')).toBeTruthy();
        expect(screen.getByText(/\+2,500/)).toBeTruthy();
        // The wallet response carries everything this screen shows, so there is
        // no extra /api/me round trip before it.
        expect(fetchMock).not.toHaveBeenCalledWith('/api/me', expect.anything());
        expect(fetchMock).toHaveBeenCalledTimes(3);
    });

    it('reports an expired session from the first real call instead of a separate /api/me probe', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/wallet') {
                return jsonResponse('unauthorized', 401);
            }
            return jsonResponse([]);
        });

        renderWithAppProviders([
            { path: '/wallet', element: <Wallet /> },
            { path: '/', element: <div>Home</div> },
        ], ['/wallet']);

        expect(await screen.findByRole('heading', { name: 'Session expired' })).toBeTruthy();
        expect(fetchMock).not.toHaveBeenCalledWith('/api/me', expect.anything());
    });

    it('shows a referral totals warning when wallet stats are unavailable', async () => {
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
            if (url === '/api/wallet') {
                return jsonResponse({
                    balance: 5000,
                    currency: 'MMK',
                    auto_renew: false,
                    auto_renew_duration: null,
                    bot_url: 'https://t.me/WavyVpnBot',
                    referral_bonus_amount: 1000,
                    referral_stats_unavailable: true,
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
        expect(screen.getByText('Referral totals are temporarily unavailable. Try again in a moment.')).toBeTruthy();
    });

    it('skips the referral list when the cached profile already proved there are none', async () => {
        sharedUserDataStore.prime('test-init-data', {
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            referral_count: 0,
            referral_earned: 0,
        });

        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/wallet') {
                return jsonResponse({
                    balance: 5000,
                    currency: 'MMK',
                    auto_renew: false,
                    auto_renew_duration: null,
                    bot_url: 'https://t.me/WavyVpnBot',
                    referral_count: 0,
                    referral_earned: 0,
                    referral_bonus_amount: 1000,
                });
            }
            if (url === '/api/wallet/history?limit=10') {
                return jsonResponse([]);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/wallet', element: <Wallet /> },
            { path: '/', element: <div>Home</div> },
        ], ['/wallet']);

        expect(await screen.findByRole('heading', { name: 'Wavy Wallet' })).toBeTruthy();
        expect(fetchMock.mock.calls.find(([url]) => String(url) === '/api/referrals')).toBeUndefined();
        expect(screen.queryByText('Referral activity is temporarily unavailable. Your totals are still shown above.')).toBeNull();
    });

    it('still loads the referral list when the cached profile holds referrals', async () => {
        sharedUserDataStore.prime('test-init-data', {
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            referral_count: 2,
            referral_earned: 2500,
        });

        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/wallet') {
                return jsonResponse({
                    balance: 5000,
                    currency: 'MMK',
                    auto_renew: false,
                    auto_renew_duration: null,
                    bot_url: 'https://t.me/WavyVpnBot',
                    referral_count: 2,
                    referral_earned: 2500,
                    referral_bonus_amount: 1000,
                });
            }
            if (url === '/api/wallet/history?limit=10') {
                return jsonResponse([]);
            }
            if (url === '/api/referrals') {
                return jsonResponse([
                    { id: 9, masked_id: '****42', created_at: '2026-01-01T00:00:00Z', status: 'pending' },
                ]);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/wallet', element: <Wallet /> },
            { path: '/', element: <div>Home</div> },
        ], ['/wallet']);

        expect(await screen.findByRole('heading', { name: 'Wavy Wallet' })).toBeTruthy();
        expect(await screen.findByText('#****42')).toBeTruthy();
    });

    it('keeps the referral screen correct when the wallet is opened directly', async () => {
        // No cached profile at all: the referral list must still be requested so a
        // deep link into /wallet shows real activity rather than an empty state.
        invalidateUserData();

        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/wallet') {
                return jsonResponse({
                    balance: 1000,
                    currency: 'MMK',
                    auto_renew: false,
                    auto_renew_duration: null,
                    referral_bonus_amount: 1000,
                });
            }
            if (url === '/api/wallet/history?limit=10') {
                return jsonResponse([]);
            }
            if (url === '/api/referrals') {
                return jsonResponse([
                    { id: 11, masked_id: '****77', created_at: '2026-02-02T00:00:00Z', status: 'bonus_received' },
                ]);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/wallet', element: <Wallet /> },
            { path: '/', element: <div>Home</div> },
        ], ['/wallet']);

        expect(await screen.findByText('#****77')).toBeTruthy();
    });
});
