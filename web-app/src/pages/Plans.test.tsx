import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Plans } from './Plans';
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

describe('Plans', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('builds wallet top-up checkout links without a negative plan index', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
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
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/plans', element: <Plans /> },
            { path: '/checkout', element: <div>Checkout</div> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/plans?walletTopup=true']);

        const links = await screen.findAllByRole('link');
        const checkoutLinks = links
            .map((link) => link.getAttribute('href'))
            .filter((href): href is string => href !== null && href.startsWith('/checkout'));

        expect(checkoutLinks[0]).toBe('/checkout?walletTopup=true&amount=5000');
        expect(checkoutLinks.every((href) => !href.includes('/checkout/-1'))).toBe(true);
    });

    it('ignores stale promo validation responses', async () => {
        let resolveFirst!: (value: Response) => void;
        let resolveSecond!: (value: Response) => void;

        fetchMock.mockImplementation((input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/plans') {
                return Promise.resolve(jsonResponse([
                    { id: 'plan-30', label: '1 Month', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                ]));
            }
            if (url === '/api/me') {
                return Promise.resolve(jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [],
                    is_active: false,
                    expire_at: null,
                    days_remaining: 0,
                    trial_eligible: false,
                    trial_days: 0,
                }));
            }
            if (url.includes('/api/promo/validate?code=SAVE10')) {
                return new Promise<Response>((resolve) => {
                    resolveFirst = resolve;
                });
            }
            if (url.includes('/api/promo/validate?code=BADCODE')) {
                return new Promise<Response>((resolve) => {
                    resolveSecond = resolve;
                });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/plans', element: <Plans /> },
            { path: '/checkout', element: <div>Checkout</div> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/plans']);

        const input = await screen.findByRole('textbox', { name: 'Enter promo code' });
        const applyButton = screen.getByRole('button', { name: 'Apply' });

        fireEvent.change(input, { target: { value: 'SAVE10' } });
        fireEvent.click(applyButton);
        await waitFor(() => expect(typeof resolveFirst).toBe('function'));

        fireEvent.change(input, { target: { value: 'BADCODE' } });
        fireEvent.click(applyButton);
        await waitFor(() => expect(typeof resolveSecond).toBe('function'));

        resolveSecond(jsonResponse('Invalid or expired code', 404));
        await screen.findByRole('alert');

        resolveFirst(jsonResponse({ valid: true, code: 'SAVE10', discount_percent: 10 }));

        await waitFor(() => {
            expect(screen.getByRole('alert').textContent).toContain('Code not found or has expired');
        });
    });

    it('shows a clearer promo section and lets users clear an applied code', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
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
            if (url.includes('/api/promo/validate?code=save10')) {
                return jsonResponse({ valid: true, code: 'SAVE10', discount_percent: 10 });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/plans', element: <Plans /> },
            { path: '/checkout', element: <div>Checkout</div> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/plans']);

        expect(await screen.findByText('Have a promo code?')).toBeTruthy();
        expect(screen.getByText('Apply it before checkout to see your discounted price.')).toBeTruthy();

        fireEvent.change(screen.getByRole('textbox', { name: 'Enter promo code' }), { target: { value: 'save10' } });
        fireEvent.click(screen.getByRole('button', { name: 'Apply' }));

        expect(await screen.findByRole('status')).toHaveTextContent('10% off your order');
        expect(screen.getByText('SAVE10')).toBeTruthy();

        fireEvent.click(screen.getByRole('button', { name: 'Clear' }));

        await waitFor(() => {
            expect(screen.queryByText('SAVE10')).toBeNull();
        });
        expect(screen.getByRole('textbox', { name: 'Enter promo code' })).toHaveValue('');
    });

    it('uses stable plan ids in checkout links and hides archived plans from customers', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/plans') {
                return jsonResponse([
                    { id: 'starter-plan', label: 'Starter', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 },
                    { id: 'legacy-plan', label: 'Legacy Plan', days: 7, price: 1500, traffic_limit_gb: 5, currency: 'MMK', active: false, sort_order: 1 },
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
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/plans', element: <Plans /> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/plans']);

        const starterLink = await screen.findByRole('link', { name: /Starter/i });
        expect(starterLink.getAttribute('href')).toBe('/checkout/starter-plan');
        expect(screen.queryByText('Legacy Plan')).toBeNull();
    });

    it('shows an empty state when no sellable plans are available', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/plans') {
                return jsonResponse([]);
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
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/plans', element: <Plans /> },
            { path: '/checkout/:planIndex', element: <div>Checkout with plan</div> },
            { path: '/wallet', element: <div>Wallet</div> },
        ], ['/plans']);

        expect(await screen.findByText('No plans available right now')).toBeTruthy();
    });
});
