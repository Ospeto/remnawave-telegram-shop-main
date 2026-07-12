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

vi.mock('../lib/twa', async () => {
    const actual = await vi.importActual<typeof import('../lib/twa')>('../lib/twa');
    return {
        ...actual,
        useTelegram: () => telegramState,
        createIdempotencyKey: () => 'test-idempotency-key',
    };
});

vi.mock('../lib/useMXBrownSound', () => ({
    useMXBrownSound: () => ({ playClick: vi.fn() }),
}));

function adminMe() {
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
            {
                telegram_id: 1001,
                is_reseller: true,
                credit_limit: 50000,
                balance_owed: 10000,
                remaining_credit: 40000,
            },
        ];

        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);
            const method = init?.method ?? 'GET';

            if (url === '/api/me') {
                return adminMe();
            }

            if (url === '/api/admin/resellers' && method === 'GET') {
                return jsonResponse(resellers);
            }

            if (url === '/api/admin/customers/2002/reseller' && method === 'PATCH') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body).toEqual({ is_reseller: true });
                const entry = {
                    telegram_id: 2002,
                    is_reseller: true,
                    credit_limit: 0,
                    balance_owed: 0,
                    remaining_credit: 0,
                };
                resellers = [...resellers.filter((r) => r.telegram_id !== 2002), entry];
                return jsonResponse(entry);
            }

            if (url === '/api/admin/customers/1001/reseller' && method === 'PATCH') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body).toEqual({ is_reseller: false });
                resellers = resellers.filter((r) => r.telegram_id !== 1001);
                return jsonResponse({
                    telegram_id: 1001,
                    is_reseller: false,
                    credit_limit: 0,
                    balance_owed: 0,
                    remaining_credit: 0,
                });
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

    it('lists credit fields when the resellers API returns them', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/me') return adminMe();
            if (url === '/api/admin/resellers') {
                return jsonResponse([
                    {
                        telegram_id: 1001,
                        is_reseller: true,
                        credit_limit: 100000,
                        balance_owed: 25000,
                        remaining_credit: 75000,
                    },
                ]);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/resellers', element: <AdminResellers /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/resellers']);

        expect(await screen.findByText('1001')).toBeTruthy();
        expect(screen.getByText(/Credit limit:\s*100,000/)).toBeTruthy();
        expect(screen.getByText(/Balance owed:\s*25,000/)).toBeTruthy();
        expect(screen.getByText(/Remaining credit:\s*75,000/)).toBeTruthy();
    });

    it('sets credit limit via PATCH with credit_limit body', async () => {
        let resellers = [
            {
                telegram_id: 1001,
                is_reseller: true,
                credit_limit: 50000,
                balance_owed: 10000,
                remaining_credit: 40000,
            },
        ];

        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);
            const method = (init?.method ?? 'GET').toUpperCase();

            if (url === '/api/me') return adminMe();
            if (url === '/api/admin/resellers' && method === 'GET') {
                return jsonResponse(resellers);
            }
            if (url === '/api/admin/customers/1001/credit' && method === 'PATCH') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body).toEqual({ credit_limit: 80000 });
                const updated = {
                    telegram_id: 1001,
                    is_reseller: true,
                    credit_limit: 80000,
                    balance_owed: 10000,
                    remaining_credit: 70000,
                };
                resellers = [updated];
                return jsonResponse(updated);
            }
            throw new Error(`Unhandled fetch: ${method} ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/resellers', element: <AdminResellers /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/resellers']);

        expect(await screen.findByText('1001')).toBeTruthy();

        fireEvent.click(screen.getByRole('button', { name: 'Manage credit' }));
        const limitInput = await screen.findByLabelText('Credit limit');
        fireEvent.change(limitInput, { target: { value: '80000' } });
        fireEvent.click(screen.getByRole('button', { name: 'Set credit limit' }));

        expect(await screen.findByText('Credit limit updated.')).toBeTruthy();

        await waitFor(() => {
            const creditCall = fetchMock.mock.calls.find(
                ([input, init]) =>
                    String(input) === '/api/admin/customers/1001/credit'
                    && String((init as RequestInit | undefined)?.method || 'GET').toUpperCase() === 'PATCH',
            );
            expect(creditCall).toBeTruthy();
            expect(JSON.parse(String((creditCall?.[1] as RequestInit).body))).toEqual({ credit_limit: 80000 });
        });

        await waitFor(() => {
            expect(screen.getByText(/Credit limit:\s*80,000/)).toBeTruthy();
        });
    });

    it('records offline AR settlement without calling wallet endpoints', async () => {
        let resellers = [
            {
                telegram_id: 1001,
                is_reseller: true,
                credit_limit: 100000,
                balance_owed: 25000,
                remaining_credit: 75000,
            },
        ];

        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);
            const method = (init?.method ?? 'GET').toUpperCase();

            if (url === '/api/me') return adminMe();
            if (url === '/api/admin/resellers' && method === 'GET') {
                return jsonResponse(resellers);
            }
            if (url === '/api/admin/customers/1001/settlements' && method === 'POST') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body.amount).toBe(15000);
                expect(body.note).toBe('Cash payment');
                expect(body.payment_method).toBeUndefined();
                expect(body.idempotency_key).toBe('test-idempotency-key');
                const headers = new Headers(init?.headers);
                expect(headers.get('Idempotency-Key')).toBe('test-idempotency-key');
                resellers = [{
                    telegram_id: 1001,
                    is_reseller: true,
                    credit_limit: 100000,
                    balance_owed: 10000,
                    remaining_credit: 90000,
                }];
                return jsonResponse({
                    created: true,
                    amount: 15000,
                    balance_owed: 10000,
                    remaining_credit: 90000,
                    idempotency_key: 'test-idempotency-key',
                    ledger_entry_id: 7,
                });
            }
            throw new Error(`Unhandled fetch: ${method} ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/resellers', element: <AdminResellers /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/resellers']);

        expect(await screen.findByText('1001')).toBeTruthy();

        fireEvent.click(screen.getByRole('button', { name: 'Manage credit' }));
        fireEvent.change(await screen.findByLabelText('Settlement amount'), { target: { value: '15000' } });
        fireEvent.change(screen.getByLabelText('Note (optional)'), { target: { value: 'Cash payment' } });
        fireEvent.click(screen.getByRole('button', { name: 'Record settlement' }));

        expect(await screen.findByText('Offline settlement recorded. Balance updated.')).toBeTruthy();

        await waitFor(() => {
            const settlementCall = fetchMock.mock.calls.find(
                ([input, init]) =>
                    String(input) === '/api/admin/customers/1001/settlements'
                    && String((init as RequestInit | undefined)?.method || 'GET').toUpperCase() === 'POST',
            );
            expect(settlementCall).toBeTruthy();
        });

        const walletCalls = fetchMock.mock.calls.filter(([input]) => {
            const url = String(input);
            return url.includes('/wallet') || url.includes('/api/reseller/settlements');
        });
        expect(walletCalls).toHaveLength(0);

        await waitFor(() => {
            expect(screen.getByText(/Balance owed:\s*10,000/)).toBeTruthy();
        });
    });

    it('loads admin ledger entries with signed amounts', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/me') return adminMe();
            if (url === '/api/admin/resellers') {
                return jsonResponse([
                    {
                        telegram_id: 1001,
                        is_reseller: true,
                        credit_limit: 100000,
                        balance_owed: 15000,
                        remaining_credit: 85000,
                    },
                ]);
            }
            if (url === '/api/admin/customers/1001/ledger') {
                return jsonResponse([
                    {
                        id: 1,
                        entry_type: 'sale',
                        direction: 'increase',
                        amount: 25000,
                        effective_at: '2026-07-01T10:00:00Z',
                        note: 'Postpaid purchase',
                        created_by: 'system',
                    },
                    {
                        id: 2,
                        entry_type: 'settlement',
                        direction: 'decrease',
                        amount: 10000,
                        effective_at: '2026-07-02T10:00:00Z',
                        note: 'Offline cash',
                        created_by: 'admin:42',
                    },
                ]);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/resellers', element: <AdminResellers /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/resellers']);

        expect(await screen.findByText('1001')).toBeTruthy();
        fireEvent.click(screen.getByRole('button', { name: 'View ledger' }));

        expect(await screen.findByText('sale')).toBeTruthy();
        expect(screen.getByText('+25,000')).toBeTruthy();
        expect(screen.getByText('settlement')).toBeTruthy();
        expect(screen.getByText('-10,000')).toBeTruthy();
        expect(screen.getByText('Offline cash')).toBeTruthy();
    });
});
