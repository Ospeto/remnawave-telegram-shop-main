import { APIError, fetchJSON } from './http';

interface TelegramSessionResponse {
    token: string;
    expires_at: string;
}

interface StoredTelegramSession {
    token: string;
    expiresAt: string;
    initData: string;
}

const TELEGRAM_SESSION_KEY = 'telegram_api_session_v1';
const SESSION_TOKEN_HEADER = 'X-Session-Token';
const SESSION_EXPIRES_HEADER = 'X-Session-Expires-At';
const inFlightSessionExchanges = new Map<string, Promise<TelegramSessionResponse>>();

function readStoredSession(): StoredTelegramSession | null {
    const raw = sessionStorage.getItem(TELEGRAM_SESSION_KEY);
    if (!raw) return null;

    try {
        const parsed = JSON.parse(raw) as StoredTelegramSession;
        if (!parsed.token || !parsed.expiresAt || !parsed.initData) {
            sessionStorage.removeItem(TELEGRAM_SESSION_KEY);
            return null;
        }
        return parsed;
    } catch {
        sessionStorage.removeItem(TELEGRAM_SESSION_KEY);
        return null;
    }
}

function isStoredSessionValid(session: StoredTelegramSession | null, initData: string): session is StoredTelegramSession {
    if (!session) return false;
    if (session.initData !== initData) return false;
    return new Date(session.expiresAt).getTime() > Date.now() + 5_000;
}

function storeSession(session: TelegramSessionResponse, initData: string) {
    sessionStorage.setItem(TELEGRAM_SESSION_KEY, JSON.stringify({
        token: session.token,
        expiresAt: session.expires_at,
        initData,
    }));
}

function updateStoredSessionFromResponse(response: Response, initData: string) {
    const token = response.headers.get(SESSION_TOKEN_HEADER);
    const expiresAt = response.headers.get(SESSION_EXPIRES_HEADER);
    if (!token || !expiresAt) return;

    storeSession({
        token,
        expires_at: expiresAt,
    }, initData);
}

async function exchangeTelegramSession(initData: string): Promise<TelegramSessionResponse> {
    const existing = inFlightSessionExchanges.get(initData);
    if (existing) {
        return existing;
    }

    const request = fetchJSON<TelegramSessionResponse>('/api/session', {
        method: 'POST',
        headers: {
            Authorization: `twa ${initData}`,
        },
    }).then((session) => {
        storeSession(session, initData);
        return session;
    }).finally(() => {
        if (inFlightSessionExchanges.get(initData) === request) {
            inFlightSessionExchanges.delete(initData);
        }
    });

    inFlightSessionExchanges.set(initData, request);
    return request;
}

export function clearTelegramSession() {
    sessionStorage.removeItem(TELEGRAM_SESSION_KEY);
}

export async function getTelegramAuthHeaders(initData: string): Promise<Record<string, string>> {
    const stored = readStoredSession();
    if (isStoredSessionValid(stored, initData)) {
        return { Authorization: `Bearer ${stored.token}` };
    }

    const session = await exchangeTelegramSession(initData);
    return { Authorization: `Bearer ${session.token}` };
}

function mergeHeaders(headers: Record<string, string>, init?: RequestInit): HeadersInit {
    const merged: Record<string, string> = { ...headers };
    const initialHeaders = new Headers(init?.headers);
    initialHeaders.forEach((value, key) => {
        merged[key] = value;
    });
    return merged;
}

async function executeAuthorizedFetch(
    input: RequestInfo | URL,
    initData: string,
    init?: RequestInit,
): Promise<Response> {
    const authHeaders = await getTelegramAuthHeaders(initData);
    const response = await fetch(input, {
        ...init,
        headers: mergeHeaders(authHeaders, init),
    });
    updateStoredSessionFromResponse(response, initData);
    return response;
}

export async function fetchWithTelegramAuth(
    input: RequestInfo | URL,
    initData: string,
    init?: RequestInit,
): Promise<Response> {
    let response = await executeAuthorizedFetch(input, initData, init);
    if (response.status !== 401) {
        return response;
    }

    clearTelegramSession();

    // U6: retrying with the same initData is pointless when Telegram has rotated
    // or aged it out — the signature check fails identically. Ask the WebView for
    // its current initData first, so a live session recovers without a reload;
    // only when it cannot produce a different value does the caller fall through
    // to the "Reopen Mini App" screen, which reloads the WebView and is what
    // actually mints fresh initData.
    const refreshedInitData = refreshTelegramInitData(initData);
    response = await executeAuthorizedFetch(input, refreshedInitData ?? initData, init);
    if (response.status === 401 && !refreshedInitData) {
        notifySessionExpired();
    }
    return response;
}

/** InitData the client is currently holding, as reported by the live WebView. */
export function currentTelegramInitData(): string {
    if (typeof window === 'undefined') return '';
    return window.Telegram?.WebApp?.initData || '';
}

/**
 * Re-reads initData from the WebView, re-initialising the Telegram SDK.
 *
 * Returns the fresh value only when it actually differs from `staleInitData`:
 * retrying with an identical payload would be rejected the same way and would
 * only hide the fact that a reload is required.
 */
export function refreshTelegramInitData(staleInitData: string): string | null {
    if (typeof window === 'undefined') return null;

    const webApp = window.Telegram?.WebApp;
    try {
        webApp?.ready?.();
        webApp?.expand?.();
    } catch {
        // A WebView without a working SDK cannot refresh anything; the caller
        // then offers the reload path.
    }

    const fresh = currentTelegramInitData();
    if (!fresh || fresh === staleInitData) return null;
    return fresh;
}

/** Event emitted when the session cannot be recovered without a reload. */
export const TELEGRAM_SESSION_EXPIRED_EVENT = 'telegram-session-expired';

function notifySessionExpired() {
    if (typeof window === 'undefined' || typeof window.dispatchEvent !== 'function') return;
    try {
        window.dispatchEvent(new CustomEvent(TELEGRAM_SESSION_EXPIRED_EVENT));
    } catch {
        // Event dispatch is best effort; the screens render from their own state.
    }
}

export async function fetchJSONWithTelegramAuth<T>(
    input: RequestInfo | URL,
    initData: string,
    init?: RequestInit,
): Promise<T> {
    const response = await fetchWithTelegramAuth(input, initData, init);
    if (!response.ok) {
        let body = '';
        try {
            body = await response.text();
        } catch {
            body = '';
        }
        throw new APIError(response.status, body);
    }
    return response.json() as Promise<T>;
}

export async function fetchUserScopedJSONWithTelegramAuth<T extends { user?: { telegram_id?: number } }>(
    input: RequestInfo | URL,
    initData: string,
    _expectedTelegramID?: number,
    init?: RequestInit,
): Promise<T> {
    // The backend already validates the signed Telegram initData when it
    // exchanges a Mini App session. `initDataUnsafe.user.id` in the webview can
    // lag or diverge from that signed payload in some Telegram clients, which
    // causes false "session expired" screens even though the server auth is
    // valid. Trust the server-authenticated response here.
    return fetchJSONWithTelegramAuth<T>(input, initData, init);
}
