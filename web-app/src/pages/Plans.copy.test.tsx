import { fireEvent, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Plans } from './Plans';
import { jsonResponse, renderWithAppProviders, seedTelegramSession } from '../test/test-utils';
import { translations } from '../lib/translations';

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

const PLAN = { id: 'plan-30', label: '1 Month', days: 30, price: 5000, traffic_limit_gb: 0, currency: 'MMK', active: true, sort_order: 0 };

function mockHappyPath() {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url === '/api/plans') return jsonResponse([PLAN]);
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
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
}

describe('Plans copy', () => {
    beforeEach(() => {
        localStorage.clear();
        seedTelegramSession();
    });

    // The duration line on the buy screen used to read "30 days left" - a
    // renewal phrasing - because the label was built by stripping the word
    // "left" from the wrong key, which did nothing at all in Burmese.
    it('states the plan duration without claiming days are running out', async () => {
        mockHappyPath();
        renderWithAppProviders([{ path: '/plans', element: <Plans /> }], ['/plans']);

        await screen.findByRole('link', { name: /1 Month/i });

        expect(screen.getByText(new RegExp(translations.en.plan_days.replace('{{days}}', '30')))).toBeInTheDocument();
        expect(screen.queryByText(/30 days left/i)).not.toBeInTheDocument();
    });

    // The per-day line used to read "167 mmk MMK/day": the currency was
    // lower-cased into a string that already carried it.
    it('states the per-day price once, without a duplicated currency code', async () => {
        mockHappyPath();
        renderWithAppProviders([{ path: '/plans', element: <Plans /> }], ['/plans']);

        await screen.findByRole('link', { name: /1 Month/i });

        const perDay = translations.en.per_day_price.replace('{{amount}}', Math.round(PLAN.price / PLAN.days).toLocaleString());
        expect(screen.getByText(new RegExp(perDay.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')))).toBeInTheDocument();
        expect(screen.queryByText(/mmk MMK\/day/i)).not.toBeInTheDocument();
    });

    it('carries the same two keys in Burmese so a Burmese customer is not shown English', () => {
        expect(translations.my.plan_days).toBeDefined();
        expect(translations.my.per_day_price).toBeDefined();
        expect(translations.my.plan_days).not.toBe(translations.en.plan_days);
        expect(translations.my.per_day_price).not.toBe(translations.en.per_day_price);
        expect(translations.my.plan_days).toContain('{{days}}');
        expect(translations.my.per_day_price).toContain('{{amount}}');
    });

    // Readiness: a customer who pays before installing Happ cannot use the key
    // they just bought, so the buy screen says what to have ready.
    it('tells the customer what to have ready before paying', async () => {
        mockHappyPath();
        renderWithAppProviders([{ path: '/plans', element: <Plans /> }], ['/plans']);

        await screen.findByRole('link', { name: /1 Month/i });

        expect(screen.getByText(translations.en.ready_title)).toBeInTheDocument();
        for (const item of ['ready_item_happ', 'ready_item_bank', 'ready_item_amount', 'ready_item_screenshot']) {
            expect(screen.getByText(translations.en[item])).toBeInTheDocument();
        }
    });

    // The switcher used to live only on Home, so a customer arriving from a
    // renewal notification could not change language without navigating home.
    it('reaches the language switcher before paying', async () => {
        mockHappyPath();
        renderWithAppProviders([{ path: '/plans', element: <Plans /> }], ['/plans']);

        await screen.findByRole('link', { name: /1 Month/i });

        const switcher = screen.getByRole('button', { name: /Switch to Myanmar/i });
        fireEvent.click(switcher);

        // The screen now reads Burmese - the same key, the other language.
        expect(await screen.findByText(translations.my.ready_title)).toBeInTheDocument();
        expect(localStorage.getItem('app_lang')).toBe('my');
    });
});
