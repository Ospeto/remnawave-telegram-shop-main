import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminPlans } from './AdminPlans';
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

describe('AdminPlans', () => {
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

    it('blocks non-admin users from plan management', async () => {
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
            { path: '/admin/plans', element: <AdminPlans /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/plans']);

        expect(await screen.findByRole('alert')).toHaveTextContent('Admin access required to manage plans.');
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it('lists, creates, edits, and archives plans', async () => {
        let plans = [
            {
                id: 'basic-30',
                label: '1 Month',
                days: 30,
                price: 5000,
                traffic_limit_gb: 0,
                currency: 'MMK',
                sort_order: 0,
                active: true,
            },
            {
                id: 'legacy-7',
                label: 'Legacy',
                days: 7,
                price: 1500,
                traffic_limit_gb: 5,
                currency: 'MMK',
                sort_order: 1,
                active: false,
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

            if (url === '/api/admin/plans' && method === 'GET') {
                return jsonResponse(plans);
            }

            if (url === '/api/admin/plans' && method === 'POST') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                const createdPlan = {
                    id: 'new-90',
                    label: body.label,
                    days: body.days,
                    price: body.price,
                    traffic_limit_gb: body.traffic_limit_gb,
                    sort_order: body.sort_order,
                    active: true,
                    currency: 'MMK',
                };
                plans = [...plans, createdPlan];
                return jsonResponse(createdPlan, 201);
            }

            if (url === '/api/admin/plans/basic-30' && method === 'PATCH') {
                const body = JSON.parse(String(init?.body ?? '{}'));
                const updatedPlan = {
                    id: 'basic-30',
                    label: body.label,
                    days: body.days,
                    price: body.price,
                    traffic_limit_gb: body.traffic_limit_gb,
                    sort_order: body.sort_order,
                    active: true,
                    currency: 'MMK',
                };
                plans = plans.map((plan) => plan.id === updatedPlan.id ? updatedPlan : plan);
                return jsonResponse(updatedPlan);
            }

            if (url === '/api/admin/plans/basic-30' && method === 'DELETE') {
                const archivedPlan = {
                    ...plans.find((plan) => plan.id === 'basic-30')!,
                    active: false,
                };
                plans = plans.map((plan) => plan.id === archivedPlan.id ? archivedPlan : plan);
                return jsonResponse('', 204);
            }

            throw new Error(`Unhandled fetch: ${method} ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin/plans', element: <AdminPlans /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin/plans']);

        expect(await screen.findByRole('heading', { name: 'Plans' })).toBeTruthy();
        expect(screen.getByText('2')).toBeTruthy();
        expect(screen.getAllByText('Archived').length).toBeGreaterThan(0);
        expect(screen.getByText('1 Month')).toBeTruthy();

        const createSection = screen.getByRole('button', { name: 'Create Plan' }).closest('section');
        if (!createSection) throw new Error('Create section not found');

        fireEvent.change(within(createSection).getByLabelText('Plan label'), { target: { value: '3 Months' } });
        fireEvent.change(within(createSection).getByLabelText('Duration days'), { target: { value: '90' } });
        fireEvent.change(within(createSection).getByLabelText('Price'), { target: { value: '12000' } });
        fireEvent.change(within(createSection).getByLabelText('Traffic limit GB'), { target: { value: '0' } });
        fireEvent.click(screen.getByRole('button', { name: 'Create Plan' }));

        expect(await screen.findByText('3 Months')).toBeTruthy();

        const planCard = screen.getByTestId('admin-plan-basic-30');
        fireEvent.change(within(planCard).getByLabelText('Plan label'), { target: { value: '1 Month Plus' } });
        fireEvent.change(within(planCard).getByLabelText('Price'), { target: { value: '5500' } });
        fireEvent.click(within(planCard).getByRole('button', { name: 'Save Changes' }));

        await waitFor(() => {
            expect(screen.getByText('1 Month Plus')).toBeTruthy();
        });

        fireEvent.click(within(screen.getByTestId('admin-plan-basic-30')).getByRole('button', { name: 'Archive Plan' }));
        expect(confirmMock).toHaveBeenCalledWith('Archive plan 1 Month Plus?');

        await waitFor(() => {
            expect(screen.getAllByText('Archived').length).toBeGreaterThan(1);
        });
    });
});
