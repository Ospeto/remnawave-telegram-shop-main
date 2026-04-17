import { screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Wallet } from './Wallet';
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
        expect(fetchMock).toHaveBeenCalledWith('/api/me', expect.anything());
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
});
