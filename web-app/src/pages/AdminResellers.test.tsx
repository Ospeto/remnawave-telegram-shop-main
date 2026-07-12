import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminResellers } from './AdminResellers';
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

describe('AdminResellers', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('blocks non-admin users from reseller management', async () => {
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
            { path: '/admin/resellers', element: <AdminResellers /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/resellers']);

        expect(await screen.findByRole('alert')).toHaveTextContent('Admin access required to manage resellers.');
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it('lists resellers and toggles reseller access by telegram id', async () => {
        let resellers = [
            { telegram_id: 1001, is_reseller: true },
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

            if (url === '/api/admin/resellers' && method === 'GET') {
                return jsonResponse(resellers);
            }

            if (url === '/api/admin/customers/2002/reseller' && method === 'PATCH') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body).toEqual({ is_reseller: true });
                const entry = { telegram_id: 2002, is_reseller: true };
                resellers = [...resellers.filter((r) => r.telegram_id !== 2002), entry];
                return jsonResponse(entry);
            }

            if (url === '/api/admin/customers/1001/reseller' && method === 'PATCH') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body).toEqual({ is_reseller: false });
                resellers = resellers.filter((r) => r.telegram_id !== 1001);
                return jsonResponse({ telegram_id: 1001, is_reseller: false });
            }

            throw new Error(`Unhandled fetch: ${method} ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/resellers', element: <AdminResellers /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/resellers']);

        expect(await screen.findByRole('heading', { name: 'Resellers' })).toBeTruthy();
        expect(screen.getByText('1001')).toBeTruthy();

        fireEvent.change(screen.getByLabelText('Telegram ID'), { target: { value: '2002' } });
        fireEvent.click(screen.getByRole('button', { name: 'Enable reseller' }));

        await waitFor(() => {
            expect(screen.getByText('2002')).toBeTruthy();
        });

        const enableCall = fetchMock.mock.calls.find(
            ([input, init]) => String(input) === '/api/admin/customers/2002/reseller'
                && (init as RequestInit | undefined)?.method === 'PATCH',
        );
        expect(enableCall).toBeTruthy();
        expect(JSON.parse(String((enableCall?.[1] as RequestInit).body))).toEqual({ is_reseller: true });

        fireEvent.change(screen.getByLabelText('Telegram ID'), { target: { value: '1001' } });
        fireEvent.click(screen.getByRole('button', { name: 'Disable reseller' }));

        await waitFor(() => {
            expect(screen.queryByText('1001')).toBeNull();
        });

        const disableCall = fetchMock.mock.calls.find(
            ([input, init]) => String(input) === '/api/admin/customers/1001/reseller'
                && (init as RequestInit | undefined)?.method === 'PATCH',
        );
        expect(disableCall).toBeTruthy();
        expect(JSON.parse(String((disableCall?.[1] as RequestInit).body))).toEqual({ is_reseller: false });
    });
});
