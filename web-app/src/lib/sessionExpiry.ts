import { useCallback, useEffect, useState } from 'react';
import { TELEGRAM_SESSION_EXPIRED_EVENT } from './auth';

/**
 * Tracks whether this screen has lost its Telegram session.
 *
 * U6: a 401 used to be visible only to whichever request noticed it, and only if
 * that request's own catch block handled it. Background work — the pending
 * purchase poll, a token refresh — could fail silently and leave the customer on
 * a screen whose buttons no longer work. This hook subscribes every screen to
 * the one signal the auth layer emits, so whatever discovers the expiry puts the
 * screen into its recoverable state.
 *
 * The returned setter is the ordinary state setter: pages keep marking the
 * session dead from their own 401 branches, so nothing regresses when the event
 * never fires (for example a WebView without CustomEvent support).
 *
 * Recovery itself is unchanged: the customer taps "Reopen", the WebView reloads
 * and Telegram mints fresh initData. No server-side auth rule is relaxed.
 */
export function useSessionExpired(): [boolean, (expired: boolean) => void] {
    const [authExpired, setAuthExpired] = useState(false);

    const markExpired = useCallback((expired: boolean) => {
        setAuthExpired(expired);
    }, []);

    useEffect(() => {
        if (typeof window === 'undefined' || typeof window.addEventListener !== 'function') {
            return;
        }
        const onExpired = () => setAuthExpired(true);
        window.addEventListener(TELEGRAM_SESSION_EXPIRED_EVENT, onExpired);
        return () => window.removeEventListener(TELEGRAM_SESSION_EXPIRED_EVENT, onExpired);
    }, []);

    return [authExpired, markExpired];
}
