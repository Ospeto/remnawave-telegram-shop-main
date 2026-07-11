import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Checkout } from './Checkout';
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

vi.mock('../lib/useMXBrownSound', () => ({
    useMXBrownSound: () => ({ playClick: vi.fn() }),
}));

describe('Checkout', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('loads wallet top-up checkout from query state and creates a top-up purchase without plan_index', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                if (body.payment_method !== 'wallet_topup') {
                    throw new Error(`Unexpected purchase body: ${JSON.stringify(body)}`);
                }

                return jsonResponse({
                    purchase_id: 99,
                    payment_phone: '09123456789',
                    amount: 5000,
                    currency: 'MMK',
                    instructions: 'Pay now',
                    invoice_type: 'wallet_topup',
                    bot_url: 'https://t.me/WavyVpnBot',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout?walletTopup=true&amount=5000']);

        const createButton = await screen.findByRole('button', { name: 'Create payment request' });
        expect(screen.queryByText('Invalid plan selected')).toBeNull();

        fireEvent.click(createButton);

        await screen.findByText('How to pay');
        await waitFor(() => {
            const purchaseCall = fetchMock.mock.calls.find(([url]) => url === '/api/purchase');
            expect(purchaseCall).toBeTruthy();

            const [, options] = purchaseCall as [string, RequestInit];
            const body = JSON.parse(String(options.body));
            const headers = new Headers(options.headers);
            expect(body.payment_method).toBe('wallet_topup');
            expect(body.amount).toBe(5000);
            expect(body.plan_index).toBeUndefined();
            expect(headers.get('Idempotency-Key')).toMatch(/^[0-9a-f-]{36}$/i);
        });
    });

    it('ignores forged URL discount for pre-purchase wallet eligibility', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 10000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 8500, currency: 'MMK' });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/0?promo=SAVE20&discount=20']);

        // Balance 8500 < full plan price 10000 → wallet disabled; URL discount must not enable pay.
        const walletButton = await screen.findByRole('button', { name: 'Pay 10,000 MMK from Wallet' });
        expect(walletButton).toBeDisabled();
        expect(screen.queryByRole('button', { name: 'Pay 8,000 MMK from Wallet' })).toBeNull();
        expect(screen.getByText('Wallet balance is not enough for wallet payment')).toBeTruthy();
    });

    it('sends promo_code from URL but never client discount or amount for service purchases', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 10000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                return jsonResponse({
                    purchase_id: 21,
                    payment_phone: '09123456789',
                    amount: 8000,
                    currency: 'MMK',
                    instructions: 'Pay now',
                    invoice_type: 'mobile_banking',
                    bot_url: 'https://t.me/WavyVpnBot',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/0?promo=SAVE20&discount=99']);

        fireEvent.click(await screen.findByRole('button', { name: 'Pay via mobile banking' }));

        await screen.findByText('How to pay');
        await waitFor(() => {
            const purchaseCall = fetchMock.mock.calls.find(([url]) => url === '/api/purchase');
            expect(purchaseCall).toBeTruthy();

            const [, options] = purchaseCall as [string, RequestInit];
            const body = JSON.parse(String(options.body));
            expect(body.promo_code).toBe('SAVE20');
            expect(body.discount).toBeUndefined();
            expect(body.amount).toBeUndefined();
        });
    });

    it('shows wallet success UI for wallet_payment invoice type without manual pay instructions', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 10000, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body.payment_method).toBe('wallet');
                return jsonResponse({
                    purchase_id: 33,
                    amount: 5000,
                    currency: 'MMK',
                    invoice_type: 'wallet_payment',
                    happ_link: 'https://happ.example/key',
                    bot_url: 'https://t.me/WavyVpnBot',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/0']);

        const walletButton = await screen.findByRole('button', { name: 'Pay 5,000 MMK from Wallet' });
        expect(walletButton).not.toBeDisabled();
        fireEvent.click(walletButton);

        expect(await screen.findByText('✅ Payment Verified!')).toBeTruthy();
        expect(screen.queryByText('How to pay')).toBeNull();
    });

    it('resumes a pending screenshot payment when the API rejects creating another one', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 10000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body.plan_id).toBe('plan-30');

                return jsonResponse({
                    code: 'pending_screenshot_payment',
                    message: 'You already have a pending screenshot payment. Please finish it before creating another one.',
                    pending_purchase: {
                        purchase_id: 55,
                        payment_phone: '09123456789',
                        payment_phones: { kpay: '09123456789' },
                        payment_providers: [{ key: 'kpay', label: 'KPay', phone: '09123456789' }],
                        amount: 30000,
                        currency: 'MMK',
                        instructions: 'Pay now',
                        invoice_type: 'wallet_topup',
                        bot_url: 'https://t.me/WavyVpnBot',
                    },
                }, 409);
            }
            if (url === '/api/purchase/cancel?id=55') {
                expect(init?.method).toBe('POST');
                return jsonResponse({
                    purchase_id: 55,
                    status: 'cancel',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/plan-30']);

        const manualButton = await screen.findByRole('button', { name: 'Pay via mobile banking' });
        fireEvent.click(manualButton);

        expect(await screen.findByText('Unfinished payment')).toBeTruthy();
        expect(screen.getByText('You already have an unfinished bank-transfer payment for 30,000 MMK. Upload its screenshot below, or cancel it to choose another plan.')).toBeTruthy();
        expect(screen.getByText('How to pay')).toBeTruthy();
        expect(screen.getByRole('button', { name: '📤 Upload Payment Screenshot' })).toBeTruthy();

        fireEvent.click(screen.getByRole('button', { name: 'Cancel this payment and choose another plan' }));

        expect(await screen.findByText('Plans')).toBeTruthy();
        await waitFor(() => {
            expect(fetchMock.mock.calls.find(([url]) => url === '/api/purchase/cancel?id=55')).toBeTruthy();
        });
    });

    it('uses the resumed purchase extension state for the success copy', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 10000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
            }
            if (url === '/api/me') {
                return jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [{ id: 777, label: 'Old key', subscription_url: 'https://example.com/old', expire_at: '2026-05-01T00:00:00Z', status: 'active' }],
                    is_active: true,
                    expire_at: '2026-05-01T00:00:00Z',
                    days_remaining: 7,
                    trial_eligible: false,
                    trial_days: 0,
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body.extend_key_id).toBe(777);

                return jsonResponse({
                    code: 'pending_screenshot_payment',
                    message: 'You already have a pending screenshot payment. Please finish it before creating another one.',
                    pending_purchase: {
                        purchase_id: 55,
                        payment_phone: '09123456789',
                        payment_phones: { kpay: '09123456789' },
                        payment_providers: [{ key: 'kpay', label: 'KPay', phone: '09123456789' }],
                        amount: 30000,
                        currency: 'MMK',
                        instructions: 'Pay now',
                        invoice_type: 'mobile_banking',
                        bot_url: 'https://t.me/WavyVpnBot',
                    },
                }, 409);
            }
            if (url === '/api/upload_screenshot?id=55') {
                return jsonResponse({
                    status: 'success',
                    message: 'Payment verified successfully!',
                    happ_link: 'happ://add/new-key',
                    redirect_url: '/redirect',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        const { container } = renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/plan-30?extend=777']);

        const manualButton = await screen.findByRole('button', { name: 'Pay via mobile banking' });
        fireEvent.click(manualButton);
        await screen.findByRole('button', { name: '📤 Upload Payment Screenshot' });

        const input = container.querySelector('input[type="file"]') as HTMLInputElement | null;
        expect(input).toBeTruthy();
        fireEvent.change(input!, {
            target: {
                files: [new File(['receipt'], 'receipt.png', { type: 'image/png' })],
            },
        });

        expect(await screen.findByText('Your new VPN key is live and ready to use. Tap below to add it to Happ.')).toBeTruthy();
        expect(screen.queryByText('Your key has been extended. Extra days and data have been added — you\'re all set.')).toBeNull();
    });

    it('shows extension success copy when the resumed pending purchase carries extend_key_id', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, init?: RequestInit) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 10000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
            }
            if (url === '/api/me') {
                return jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [{ id: 777, label: 'Old key', subscription_url: 'https://example.com/old', expire_at: '2026-05-01T00:00:00Z', status: 'active' }],
                    is_active: true,
                    expire_at: '2026-05-01T00:00:00Z',
                    days_remaining: 7,
                    trial_eligible: false,
                    trial_days: 0,
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                expect(body.extend_key_id).toBeUndefined();

                return jsonResponse({
                    code: 'pending_screenshot_payment',
                    message: 'You already have a pending screenshot payment. Please finish it before creating another one.',
                    pending_purchase: {
                        purchase_id: 56,
                        payment_phone: '09123456789',
                        payment_phones: { kpay: '09123456789' },
                        payment_providers: [{ key: 'kpay', label: 'KPay', phone: '09123456789' }],
                        amount: 30000,
                        currency: 'MMK',
                        instructions: 'Pay now',
                        invoice_type: 'mobile_banking',
                        extend_key_id: 777,
                        bot_url: 'https://t.me/WavyVpnBot',
                    },
                }, 409);
            }
            if (url === '/api/upload_screenshot?id=56') {
                return jsonResponse({
                    status: 'success',
                    message: 'Payment verified successfully!',
                    happ_link: 'happ://add/existing-key',
                    redirect_url: '/redirect',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        const { container } = renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/plan-30']);

        const manualButton = await screen.findByRole('button', { name: 'Pay via mobile banking' });
        fireEvent.click(manualButton);
        await screen.findByRole('button', { name: '📤 Upload Payment Screenshot' });

        const input = container.querySelector('input[type="file"]') as HTMLInputElement | null;
        expect(input).toBeTruthy();
        fireEvent.change(input!, {
            target: {
                files: [new File(['receipt'], 'receipt.png', { type: 'image/png' })],
            },
        });

        expect(await screen.findByText('Your key has been extended. Extra days and data have been added — you\'re all set.')).toBeTruthy();
        expect(screen.queryByText('Your new VPN key is live and ready to use. Tap below to add it to Happ.')).toBeNull();
    });

    it('resolves stable plan ids and sends plan_id for new checkout flows', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, _init?: RequestInit) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'starter-plan', label: 'Starter', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                return jsonResponse({
                    purchase_id: 7,
                    payment_phone: '09123456789',
                    amount: 5000,
                    currency: 'MMK',
                    instructions: 'Pay now',
                    invoice_type: 'manual',
                    bot_url: 'https://t.me/WavyVpnBot',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/starter-plan']);

        const manualButton = await screen.findByRole('button', { name: 'Pay via mobile banking' });
        fireEvent.click(manualButton);

        await screen.findByText('How to pay');
        await waitFor(() => {
            const purchaseCall = fetchMock.mock.calls.find(([url]) => url === '/api/purchase');
            expect(purchaseCall).toBeTruthy();

            const [, options] = purchaseCall as [string, RequestInit];
            const body = JSON.parse(String(options.body));
            expect(body.plan_id).toBe('starter-plan');
            expect(body.plan_index).toBeUndefined();
        });
    });

    it('keeps legacy numeric checkout routes working with stable sort_order fallback', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL, _init?: RequestInit) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'starter-plan', label: 'Starter', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                    { id: 'pro-plan', label: 'Pro', days: 90, price: 12000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 2 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }
            if (url === '/api/purchase') {
                return jsonResponse({
                    purchase_id: 8,
                    payment_phone: '09123456789',
                    amount: 12000,
                    currency: 'MMK',
                    instructions: 'Pay now',
                    invoice_type: 'manual',
                    bot_url: 'https://t.me/WavyVpnBot',
                });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/2']);

        const manualButton = await screen.findByRole('button', { name: 'Pay via mobile banking' });
        fireEvent.click(manualButton);

        await screen.findByText('How to pay');
        await waitFor(() => {
            const purchaseCall = fetchMock.mock.calls.find(([url]) => url === '/api/purchase');
            expect(purchaseCall).toBeTruthy();

            const [, options] = purchaseCall as [string, RequestInit];
            const body = JSON.parse(String(options.body));
            expect(body.plan_index).toBe(2);
            expect(body.plan_id).toBeUndefined();
        });
    });

    it('rejects legacy numeric routes when the original slot is now archived instead of remapping', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);

            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'starter-plan', label: 'Starter', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                    { id: 'pro-plan', label: 'Pro', days: 90, price: 12000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 2 },
                ]);
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
                });
            }
            if (url === '/api/wallet') {
                return jsonResponse({ balance: 0, currency: 'MMK' });
            }

            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/checkout/:planIndex', element: <Checkout /> },
            { path: '/plans', element: <div>Plans</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/checkout/1']);

        expect(await screen.findByText('Invalid plan selected')).toBeTruthy();
        expect(fetchMock.mock.calls.find(([url]) => url === '/api/purchase')).toBeUndefined();
    });
});
