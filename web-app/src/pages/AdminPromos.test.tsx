import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminPromos } from './AdminPromos';
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

describe('AdminPromos', () => {
    const fetchMock = vi.fn();
    const confirmMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        confirmMock.mockReset();
        confirmMock.mockReturnValue(true);
        vi.stubGlobal('fetch', fetchMock);
        vi.stubGlobal('confirm', confirmMock);
        seedTelegramSession();
    });

    it('blocks non-admin users from promo management', async () => {
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
            { path: '/admin/promos', element: <AdminPromos /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/promos']);

        expect(await screen.findByRole('alert')).toHaveTextContent('Admin access required to manage promos.');
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it('lists, creates, and deletes promo codes', async () => {
        let promos = [
            {
                code: 'SAVE10',
                discount_percent: 10,
                max_uses: 100,
                used_count: 1,
                valid_until: '2099-01-31T00:00:00.000Z',
                created_at: '2099-01-01T00:00:00.000Z',
                status: 'active',
            },
        ];

        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);
            const method = init?.method ?? 'GET';

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

            if (url === '/api/admin/promos' && method === 'GET') {
                return jsonResponse(promos);
            }

            if (url === '/api/admin/promos' && method === 'POST') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                const createdPromo = {
                    code: body.code,
                    discount_percent: body.discount_percent,
                    max_uses: body.max_uses,
                    used_count: 0,
                    valid_until: '2099-03-01T00:00:00.000Z',
                    created_at: '2099-02-01T00:00:00.000Z',
                    status: 'active',
                };
                promos = [createdPromo, ...promos];
                return jsonResponse(createdPromo, 201);
            }

            if (url === '/api/admin/promos/SAVE10' && method === 'DELETE') {
                promos = promos.filter((promo) => promo.code !== 'SAVE10');
                return jsonResponse('', 204);
            }

            throw new Error(`Unhandled fetch: ${method} ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/promos', element: <AdminPromos /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/promos']);

        expect(await screen.findByRole('heading', { name: 'Promo Codes' })).toBeTruthy();
        expect(screen.getByText('SAVE10')).toBeTruthy();

        fireEvent.change(screen.getByLabelText('Promo code'), { target: { value: 'FLASH25' } });
        fireEvent.change(screen.getByLabelText('Discount %'), { target: { value: '25' } });
        fireEvent.change(screen.getByLabelText('Valid days'), { target: { value: '14' } });
        fireEvent.change(screen.getByLabelText('Max uses'), { target: { value: '50' } });
        fireEvent.click(screen.getByRole('button', { name: 'Create Promo' }));

        await waitFor(() => {
            expect(screen.getByText('FLASH25')).toBeTruthy();
        });

        fireEvent.click(screen.getByRole('button', { name: 'Delete SAVE10' }));
        expect(confirmMock).toHaveBeenCalledWith('Delete promo code SAVE10?');

        await waitFor(() => {
            expect(screen.queryByText('SAVE10')).toBeNull();
        });
    });

    it('shows the session expired screen when admin promo loading gets a 401', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);

            if (url === '/api/session') {
                return jsonResponse({
                    token: 'renewed-session-token',
                    expires_at: '2999-01-01T00:00:00.000Z',
                });
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
                    is_admin: true,
                });
            }

            if (url === '/api/admin/promos') {
                return jsonResponse('Unauthorized', 401);
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/promos', element: <AdminPromos /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/promos']);

        expect(await screen.findByRole('heading', { name: 'Session expired' })).toBeTruthy();
    });
});
