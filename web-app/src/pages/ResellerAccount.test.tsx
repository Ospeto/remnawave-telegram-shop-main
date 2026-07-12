import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ResellerAccount } from './ResellerAccount';
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

describe('ResellerAccount', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('renders balances from the account API', async () => {
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
                    is_reseller: true,
                });
            }
            if (url === '/api/reseller/account') {
                return jsonResponse({
                    credit_limit: 100000,
                    balance_owed: 25000,
                    remaining_credit: 75000,
                    is_reseller: true,
                });
            }
            if (url.startsWith('/api/reseller/ledger')) {
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
                        note: 'Wallet payment',
                        created_by: 'system',
                    },
                    {
                        id: 3,
                        entry_type: 'adjustment',
                        direction: 'decrease',
                        amount: 5000,
                        effective_at: '2026-07-03T10:00:00Z',
                        note: 'Credit adjustment',
                        created_by: 'admin',
                    },
                ]);
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/reseller/account', element: <ResellerAccount /> },
            { path: '/', element: <div>Home</div> },
        ], ['/reseller/account']);

        expect(await screen.findByRole('heading', { name: 'Reseller account' })).toBeTruthy();
        expect(screen.getByText('100,000')).toBeTruthy();
        expect(screen.getByText('25,000')).toBeTruthy();
        expect(screen.getByText('75,000')).toBeTruthy();
        expect(screen.getByText('sale')).toBeTruthy();
        expect(screen.getByText('Postpaid purchase')).toBeTruthy();
        // increase → positive/red; decrease → negative/green
        expect(screen.getByText('+25,000')).toBeTruthy();
        expect(screen.getByText('-10,000')).toBeTruthy();
        expect(screen.getByText('-5,000')).toBeTruthy();
        expect(screen.getByText('settlement')).toBeTruthy();
        expect(screen.getByText('adjustment')).toBeTruthy();
        expect(screen.getByText('+25,000').style.color).toBe('rgb(255, 59, 48)');
        expect(screen.getByText('-10,000').style.color).toBe('rgb(52, 199, 89)');
        expect(screen.getByText('-5,000').style.color).toBe('rgb(52, 199, 89)');
    });

    it('pays balance via wallet settlement POST', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);
            const method = (init?.method || 'GET').toUpperCase();

            if (url === '/api/me') {
                return jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [],
                    is_active: false,
                    expire_at: null,
                    days_remaining: 0,
                    trial_eligible: false,
                    trial_days: 0,
                    is_reseller: true,
                });
            }
            if (url === '/api/reseller/account' && method === 'GET') {
                return jsonResponse({
                    credit_limit: 100000,
                    balance_owed: 25000,
                    remaining_credit: 75000,
                    is_reseller: true,
                });
            }
            if (url.startsWith('/api/reseller/ledger')) {
                return jsonResponse([]);
            }
            if (url === '/api/reseller/settlements' && method === 'POST') {
                const body = JSON.parse(String(init?.body || '{}'));
                expect(body.payment_method).toBe('wallet');
                expect(body.amount).toBe(25000);
                const headers = new Headers(init?.headers);
                expect(headers.get('Idempotency-Key')).toBe('test-idempotency-key');
                return jsonResponse({
                    created: true,
                    amount: 25000,
                    balance_owed: 0,
                    remaining_credit: 100000,
                    idempotency_key: 'test-idempotency-key',
                    ledger_entry_id: 99,
                });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/reseller/account', element: <ResellerAccount /> },
            { path: '/', element: <div>Home</div> },
        ], ['/reseller/account']);

        expect(await screen.findByRole('heading', { name: 'Reseller account' })).toBeTruthy();

        const amountInput = screen.getByLabelText('Amount') as HTMLInputElement;
        expect(amountInput.value).toBe('25000');

        fireEvent.click(screen.getByRole('button', { name: 'Pay from wallet' }));

        expect(await screen.findByText('Payment recorded. Balance updated.')).toBeTruthy();

        await waitFor(() => {
            const settlementCall = fetchMock.mock.calls.find(
                ([input, init]) =>
                    String(input) === '/api/reseller/settlements' &&
                    String((init as RequestInit | undefined)?.method || 'GET').toUpperCase() === 'POST',
            );
            expect(settlementCall).toBeTruthy();
        });
    });

    it('shows access message for non-resellers and hides settlement UI', async () => {
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
                    is_reseller: false,
                });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/reseller/account', element: <ResellerAccount /> },
            { path: '/', element: <div>Home</div> },
        ], ['/reseller/account']);

        expect(await screen.findByText('Reseller access required to view this page.')).toBeTruthy();
        expect(screen.queryByRole('button', { name: 'Pay from wallet' })).toBeNull();
        expect(screen.queryByLabelText('Amount')).toBeNull();
        expect(fetchMock.mock.calls.find(([url]) => String(url) === '/api/reseller/account')).toBeUndefined();
    });
});
