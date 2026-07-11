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

    it('shows an admin promo card only for admins', async () => {
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
            { path: '/admin/plans', element: <div>Plan Admin</div> },
            { path: '/admin/promos', element: <div>Promo Admin</div> },
            { path: '/admin/finance', element: <div>Finance Admin</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.getByRole('link', { name: /Plans Add, edit, and archive plan pricing/i })).toBeTruthy();
        expect(screen.getByRole('link', { name: /Promo Codes/i })).toBeTruthy();
        expect(screen.getByRole('link', { name: /Plans/i })).toBeTruthy();
        expect(screen.getByRole('link', { name: /Finance/i })).toBeTruthy();
    });

    it('hides the admin promo card for non-admin users', async () => {
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
            { path: '/admin/plans', element: <div>Plan Admin</div> },
            { path: '/admin/promos', element: <div>Promo Admin</div> },
            { path: '/admin/finance', element: <div>Finance Admin</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.queryByRole('link', { name: /Plans Add, edit, and archive plan pricing/i })).toBeNull();
        expect(screen.queryByRole('link', { name: /Promo Codes/i })).toBeNull();
        expect(screen.queryByRole('link', { name: /Plans/i })).toBeNull();
        expect(screen.queryByRole('link', { name: /Finance/i })).toBeNull();
    });

    it('shows Finance admin card for admins', async () => {
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
            { path: '/admin/plans', element: <div>Plan Admin</div> },
            { path: '/admin/promos', element: <div>Promo Admin</div> },
            { path: '/admin/finance', element: <div>Finance Admin</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.getByRole('link', { name: /Finance/i })).toBeTruthy();
    });

    it('hides Finance admin card for non-admin users', async () => {
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
            { path: '/admin/plans', element: <div>Plan Admin</div> },
            { path: '/admin/promos', element: <div>Promo Admin</div> },
            { path: '/admin/finance', element: <div>Finance Admin</div> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
        expect(screen.queryByRole('link', { name: /Finance/i })).toBeNull();
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
