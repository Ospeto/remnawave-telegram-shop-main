import { screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminHub } from './AdminHub';
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

describe('AdminHub', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        telegramState.tg.BackButton.show.mockReset();
        telegramState.tg.BackButton.onClick.mockReset();
        telegramState.tg.BackButton.offClick.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('blocks non-admin users from the hub', async () => {
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
            { path: '/admin', element: <AdminHub /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin']);

        expect(await screen.findByRole('alert')).toHaveTextContent('Admin access required.');
        expect(screen.queryByRole('link', { name: /Finance/i })).toBeNull();
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it('shows Shop section and three tool links for admins', async () => {
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
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin', element: <AdminHub /> },
            { path: '/admin/finance', element: <div>Finance</div> },
            { path: '/admin/plans', element: <div>Plans</div> },
            { path: '/admin/promos', element: <div>Promos</div> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin']);

        expect(await screen.findByRole('heading', { name: 'Admin' })).toBeTruthy();
        expect(screen.getByText('Shop')).toBeTruthy();
        expect(screen.getByRole('link', { name: /Finance/i })).toHaveAttribute('href', '/admin/finance');
        expect(screen.getByRole('link', { name: /Plans/i })).toHaveAttribute('href', '/admin/plans');
        expect(screen.getByRole('link', { name: /Promo Codes/i })).toHaveAttribute('href', '/admin/promos');
    });

    it('registers Telegram BackButton to navigate home', async () => {
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
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin', element: <AdminHub /> },
            { path: '/', element: <div>Home Page</div> },
        ], ['/admin']);

        await screen.findByRole('heading', { name: 'Admin' });

        expect(telegramState.tg.BackButton.show).toHaveBeenCalled();
        expect(telegramState.tg.BackButton.onClick).toHaveBeenCalled();

        const handler = telegramState.tg.BackButton.onClick.mock.calls[0][0] as () => void;
        handler();

        await waitFor(() => {
            expect(screen.getByText('Home Page')).toBeTruthy();
        });
    });
});
