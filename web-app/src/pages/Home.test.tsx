import { fireEvent, screen, waitFor, within } from '@testing-library/react';
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

    it('shows Remove button only on expired keys and opens confirmation modal', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [
                {
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
                },
                {
                    id: 102,
                    label: 'Expired Key',
                    username: 'user2',
                    subscription_url: 'https://example.com/sub/expired',
                    status: 'expired',
                    expire_at: '2020-01-01T00:00:00Z',
                    days_remaining: 0,
                    total_days: 30,
                    traffic_used_gb: 5,
                    traffic_limit_gb: 10,
                    auto_renew: false,
                },
                {
                    id: 0,
                    label: 'Legacy Key',
                    username: 'user3',
                    subscription_url: 'https://example.com/sub/legacy',
                    status: 'expired',
                    expire_at: '2020-01-01T00:00:00Z',
                    days_remaining: 0,
                    total_days: 30,
                    traffic_used_gb: 0,
                    traffic_limit_gb: 0,
                    auto_renew: false,
                },
            ],
            is_active: true,
            expire_at: null,
            days_remaining: 5,
            trial_eligible: false,
            trial_days: 0,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
        ], ['/']);

        expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();

        // Remove button should appear for expired key (id 102) but not for active key or legacy id 0
        const removeButtons = screen.getAllByRole('button', { name: 'Remove' });
        expect(removeButtons).toHaveLength(1);

        // Clicking Remove opens the confirmation modal
        fireEvent.click(removeButtons[0]);
        expect(await screen.findByRole('dialog')).toBeTruthy();
        expect(screen.getByText('Remove Expired Key')).toBeTruthy();
        expect(screen.getByText('Are you sure you want to remove this expired key?')).toBeTruthy();

        // Canceling modal closes dialog without calling any API
        const cancelButton = screen.getByRole('button', { name: 'Cancel' });
        fireEvent.click(cancelButton);
        await waitFor(() => expect(screen.queryByText('Remove Expired Key')).toBeNull());
        expect(fetchMock).toHaveBeenCalledTimes(1); // Only initial /api/me

        // Re-open and verify Escape cancellation via dialog cancel event
        fireEvent.click(removeButtons[0]);
        const dialog = await screen.findByRole('dialog');
        fireEvent(dialog, new Event('cancel', { bubbles: true, cancelable: true }));
        await waitFor(() => expect(screen.queryByText('Remove Expired Key')).toBeNull());
    });

    it('successfully confirms removal and removes key card from DOM', async () => {
        fetchMock
            .mockResolvedValueOnce(jsonResponse({
                user: { id: 1, telegram_id: 42 },
                keys: [
                    {
                        id: 102,
                        label: 'Key To Remove',
                        username: 'user2',
                        subscription_url: 'https://example.com/sub/expired',
                        status: 'expired',
                        expire_at: '2020-01-01T00:00:00Z',
                        days_remaining: 0,
                        total_days: 30,
                        traffic_used_gb: 5,
                        traffic_limit_gb: 10,
                        auto_renew: false,
                    },
                ],
                is_active: false,
                expire_at: null,
                days_remaining: 0,
                trial_eligible: false,
                trial_days: 0,
            }))
            .mockResolvedValueOnce(jsonResponse({ success: true, key_id: 102 }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
        ], ['/']);

        expect(await screen.findByText('Key To Remove')).toBeTruthy();
        const removeButton = await screen.findByRole('button', { name: 'Remove' });
        removeButton.focus();
        fireEvent.click(removeButton);

        const dialog = await screen.findByRole('dialog');
        const confirmButton = within(dialog).getByRole('button', { name: 'Remove' });
        fireEvent.click(confirmButton);

        // Modal should close, key should disappear from DOM, and heading should receive focus
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
        await waitFor(() => expect(screen.queryByText('Key To Remove')).toBeNull());
        await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('heading', { level: 1 })));

        // Verify API was called with POST to /api/keys/102/hide
        expect(fetchMock).toHaveBeenCalledTimes(2);
        expect(fetchMock.mock.calls[1][0]).toBe('/api/keys/102/hide');
        expect(fetchMock.mock.calls[1][1]?.method).toBe('POST');
    });

    it('retains key card and displays error when removal fails', async () => {
        fetchMock
            .mockResolvedValueOnce(jsonResponse({
                user: { id: 1, telegram_id: 42 },
                keys: [
                    {
                        id: 102,
                        label: 'Key To Keep',
                        username: 'user2',
                        subscription_url: 'https://example.com/sub/expired',
                        status: 'expired',
                        expire_at: '2020-01-01T00:00:00Z',
                        days_remaining: 0,
                        total_days: 30,
                        traffic_used_gb: 5,
                        traffic_limit_gb: 10,
                        auto_renew: false,
                    },
                ],
                is_active: false,
                expire_at: null,
                days_remaining: 0,
                trial_eligible: false,
                trial_days: 0,
            }))
            .mockResolvedValueOnce(jsonResponse('Renewal in progress', 409));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
        ], ['/']);

        expect(await screen.findByText('Key To Keep')).toBeTruthy();
        const removeButton = await screen.findByRole('button', { name: 'Remove' });
        fireEvent.click(removeButton);

        const dialog = await screen.findByRole('dialog');
        const confirmButton = within(dialog).getByRole('button', { name: 'Remove' });
        fireEvent.click(confirmButton);

        // Dialog should remain open with error message, key card still in DOM
        expect(await screen.findByRole('alert')).toHaveTextContent('Renewal in progress');
        expect(screen.getByText('Key To Keep')).toBeTruthy();
    });

    it('does not show Remove button for indeterminate expired key with null expire_at', async () => {
        fetchMock.mockResolvedValueOnce(jsonResponse({
            user: { id: 1, telegram_id: 42 },
            keys: [
                {
                    id: 103,
                    label: 'Indeterminate Key',
                    username: 'user4',
                    subscription_url: 'https://example.com/sub/indeterminate',
                    status: 'expired',
                    expire_at: null,
                    days_remaining: 0,
                    total_days: 30,
                    traffic_used_gb: 0,
                    traffic_limit_gb: 0,
                    auto_renew: false,
                },
            ],
            is_active: false,
            expire_at: null,
            days_remaining: 0,
            trial_eligible: false,
            trial_days: 0,
        }));

        renderWithAppProviders([
            { path: '/', element: <Home /> },
        ], ['/']);

        expect(await screen.findByText('Indeterminate Key')).toBeTruthy();
        expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull();
    });
});
