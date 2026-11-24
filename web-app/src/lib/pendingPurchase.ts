import { useEffect, useRef, useState } from 'react';
import { fetchJSONWithTelegramAuth } from './auth';
import { PendingPurchase } from './types';

/**
 * How often the Mini App re-reads an unfinished order while the screen is open.
 *
 * U3: before this, a customer who left a screenshot payment unfinished saw a
 * shop with nothing pending, and the only way to discover the order was to walk
 * into Checkout and hit the 409. The banner now resolves itself: the moment the
 * shop finishes verifying, the order stops being pending without a reload.
 */
export const PENDING_PURCHASE_POLL_MS = 5_000;

const TERMINAL_STATUSES = new Set(['paid', 'cancel', 'cancelled', 'failed', 'refunded']);

interface PurchaseStatusResponse {
    id: number;
    status: string;
}

const isVisible = () => typeof document === 'undefined' || document.visibilityState !== 'hidden';

/**
 * Tracks the customer's unfinished order and polls its status while the screen
 * is visible. Polling stops on a terminal status, while the tab is hidden, and
 * whenever the caller passes a null `initData`.
 */
export function usePendingPurchase(
    initial: PendingPurchase | null | undefined,
    initData: string,
    onResolved?: () => void,
): PendingPurchase | null {
    const [pending, setPending] = useState<PendingPurchase | null>(initial ?? null);
    const onResolvedRef = useRef(onResolved);
    onResolvedRef.current = onResolved;

    // A new profile read (navigation, refresh) is the newest truth about what is
    // in flight; the local copy must not outlive it.
    useEffect(() => {
        setPending(initial ?? null);
    }, [initial?.id, initial?.status]);

    const purchaseID = pending?.id ?? null;
    const status = pending?.status ?? null;

    useEffect(() => {
        if (!initData || !purchaseID || !status || TERMINAL_STATUSES.has(status)) return;

        let cancelled = false;

        const poll = async () => {
            if (!isVisible()) return;
            try {
                const current = await fetchJSONWithTelegramAuth<PurchaseStatusResponse>(
                    `/api/purchase/status?id=${purchaseID}`,
                    initData,
                );
                if (cancelled || !current?.status) return;
                if (TERMINAL_STATUSES.has(current.status)) {
                    setPending(null);
                    onResolvedRef.current?.();
                    return;
                }
                setPending((previous) => (previous && previous.status !== current.status
                    ? { ...previous, status: current.status, awaiting_screenshot: current.status !== 'processing' }
                    : previous));
            } catch {
                // A status probe is best effort: a failed poll leaves the banner as
                // it is and the next tick tries again.
            }
        };

        const timer = setInterval(() => { void poll(); }, PENDING_PURCHASE_POLL_MS);
        // Poll once immediately: the profile that produced the banner may already be
        // a few seconds old, and a customer who comes back to a settled order should
        // not stare at a stale "waiting" message for a whole interval.
        void poll();
        const onVisibilityChange = () => {
            if (isVisible()) void poll();
        };
        document.addEventListener?.('visibilitychange', onVisibilityChange);

        return () => {
            cancelled = true;
            clearInterval(timer);
            document.removeEventListener?.('visibilitychange', onVisibilityChange);
        };
    }, [initData, purchaseID, status]);

    return pending;
}
