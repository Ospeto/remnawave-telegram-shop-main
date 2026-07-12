import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Home } from './Home';
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

describe('Home', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('recovers after retrying a failed /api/me request', async () => {
        fetchMock
            .mockRejectedValueOnce(new Error('boom'))
            .mockResolvedValueOnce(jsonResponse({
                user: { id: 1, telegram_id: 42 },
                keys: [],
                is_active: false,
                expire_at: null,
                days_remaining: 0,
                trial_eligible: false,
                trial_days: 0,
                referral_count: 0,
                referral_earned: 0,
            }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/']);

        const retryButton = await screen.findByRole('button', { name: 'Try Again' });
        fireEvent.click(retryButton);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
        expect(screen.queryByText(/Error:/)).toBeNull();
    });

    it('hides the referral chip when referral stats are unavailable', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            referral_stats_unavailable: true,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.queryByText(/Referrals/)).toBeNull();
    });

    it('shows a single Admin card linking to /admin for admins', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            is_admin: true,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
            { path: '/admin', element: <div>Admin Hub</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        const adminLink = screen.getByRole('link', { name: /Admin Finance, plans, and promos/i });
        expect(adminLink).toHaveAttribute('href', '/admin');
        expect(screen.queryByRole('link', { name: /Promo Codes/i })).toBeNull();
        expect(screen.queryByRole('link', { name: /Plans Add, edit, and archive plan pricing/i })).toBeNull();
        // Finance tool card title must not appear as its own Home link
        expect(screen.queryByRole('link', { name: /^Finance Income/i })).toBeNull();
    });

    it('hides the Admin card for non-admin users', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            is_admin: false,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
            { path: '/admin', element: <div>Admin Hub</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.queryByRole('link', { name: /Admin Finance, plans, and promos/i })).toBeNull();
        expect(screen.queryByRole('link', { name: /Promo Codes/i })).toBeNull();
        expect(screen.queryByRole('link', { name: /Plans Add, edit, and archive plan pricing/i })).toBeNull();
    });

    it('shows Resellers admin card only for admins', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            is_admin: true,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
            { path: '/admin/resellers', element: <div>Reseller Admin</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.getByRole('link', { name: /Resellers Toggle wholesale access by Telegram ID/i })).toBeTruthy();
    });

    it('hides Resellers admin card for non-admin users', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            is_admin: false,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
            { path: '/admin/resellers', element: <div>Reseller Admin</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.queryByRole('link', { name: /Resellers/i })).toBeNull();
    });

    it('shows reseller account card when is_reseller is true', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            is_admin: false,
            is_reseller: true,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
            { path: '/reseller/account', element: <div>Reseller Account</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        const link = screen.getByRole('link', { name: /Reseller account Credit, balance owed, and pay/i });
        expect(link).toHaveAttribute('href', '/reseller/account');
    });

    it('hides reseller account card when is_reseller is false', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
            is_admin: false,
            is_reseller: false,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
            { path: '/reseller/account', element: <div>Reseller Account</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.queryByRole('link', { name: /Reseller account/i })).toBeNull();
    });

    it('shows a trial error when activation returns conflict', async () => {
        fetchMock
            .mockResolvedValueOnce(jsonResponse({
                user: { id: 1, telegram_id: 42 },
                keys: [],
                is_active: false,
                expire_at: null,
                days_remaining: 0,
                trial_eligible: true,
                trial_days: 7,
                is_admin: false,
            }))
            .mockResolvedValueOnce(jsonResponse('Trial already used', 409));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/']);

        const trialButton = await screen.findByRole('button', { name: /Start Free Trial/i });
        fireEvent.click(trialButton);

        expect(await screen.findByRole('alert')).toHaveTextContent('Trial already used');
        expect(fetchMock).toHaveBeenCalledTimes(2);
    });
});
