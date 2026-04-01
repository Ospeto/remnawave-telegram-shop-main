import { fireEvent, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { Home } from './Home';
import { jsonResponse, renderWithAppProviders } from '../test/test-utils';

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
});
