import { renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { PENDING_PURCHASE_POLL_MS, usePendingPurchase } from './pendingPurchase';
import { PendingPurchase } from './types';
import { clearTelegramSession } from './auth';
import { jsonResponse, seedTelegramSession } from '../test/test-utils';

const pending: PendingPurchase = {
    id: 77,
    status: 'pending',
    amount: 15000,
    currency: 'MMK',
    invoice_type: 'mobile_banking',
    created_at: '2026-11-24T10:00:00Z',
    awaiting_screenshot: true,
};

describe('usePendingPurchase', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
        clearTelegramSession();
        seedTelegramSession();
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    // U3: the order was invisible unless the customer happened to start another
    // purchase; the hook is what keeps the banner honest while the screen is open.
    it('exposes the pending purchase it was given', () => {
        const { result } = renderHook(() => usePendingPurchase(pending, 'test-init-data'));

        expect(result.current?.id).toBe(77);
        expect(result.current?.status).toBe('pending');
    });

    it('polls /api/purchase/status and clears the banner once the order settles', async () => {
        fetchMock.mockResolvedValue(jsonResponse({ id: 77, status: 'paid' }));
        const onResolved = vi.fn();

        const { result } = renderHook(() => usePendingPurchase(pending, 'test-init-data', onResolved));

        await waitFor(() => expect(result.current).toBeNull(), { timeout: PENDING_PURCHASE_POLL_MS + 2_000 });
        expect(fetchMock).toHaveBeenCalledWith(
            '/api/purchase/status?id=77',
            expect.objectContaining({ headers: expect.anything() }),
        );
        expect(onResolved).toHaveBeenCalled();
    });

    it('keeps the banner and follows a status change while the order is still open', async () => {
        fetchMock.mockResolvedValue(jsonResponse({ id: 77, status: 'processing' }));

        const { result } = renderHook(() => usePendingPurchase(pending, 'test-init-data'));

        await waitFor(() => expect(result.current?.status).toBe('processing'), {
            timeout: PENDING_PURCHASE_POLL_MS + 2_000,
        });
        expect(result.current?.awaiting_screenshot).toBe(false);
    });
});
