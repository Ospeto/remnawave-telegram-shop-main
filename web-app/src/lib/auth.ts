import { APIError, fetchJSON } from './http';

interface TelegramSessionResponse {
    token: string;
    expires_at: string;
}

interface StoredTelegramSession {
    token: string;
    expiresAt: string;
}

const TELEGRAM_SESSION_KEY = 'telegram_api_session_v1';

function readStoredSession(): StoredTelegramSession | null {
    const raw = sessionStorage.getItem(TELEGRAM_SESSION_KEY);
    if (!raw) return null;

    try {
        const parsed = JSON.parse(raw) as StoredTelegramSession;
        if (!parsed.token || !parsed.expiresAt) {
            sessionStorage.removeItem(TELEGRAM_SESSION_KEY);
            return null;
        }
        return parsed;
    } catch {
        sessionStorage.removeItem(TELEGRAM_SESSION_KEY);
        return null;
    }
}

function isStoredSessionValid(session: StoredTelegramSession | null): session is StoredTelegramSession {
    if (!session) return false;
    return new Date(session.expiresAt).getTime() > Date.now() + 5_000;
}

function storeSession(session: TelegramSessionResponse) {
    sessionStorage.setItem(TELEGRAM_SESSION_KEY, JSON.stringify({
        token: session.token,
        expiresAt: session.expires_at,
    }));
}

async function exchangeTelegramSession(initData: string): Promise<TelegramSessionResponse> {
    const session = await fetchJSON<TelegramSessionResponse>('/api/session', {
        method: 'POST',
        headers: {
            Authorization: `twa ${initData}`,
        },
    });
    storeSession(session);
    return session;
}

export function clearTelegramSession() {
    sessionStorage.removeItem(TELEGRAM_SESSION_KEY);
}

export async function getTelegramAuthHeaders(initData: string): Promise<Record<string, string>> {
    const stored = readStoredSession();
    if (isStoredSessionValid(stored)) {
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

export async function fetchJSONWithTelegramAuth<T>(
    input: RequestInfo | URL,
    initData: string,
    init?: RequestInit,
): Promise<T> {
    const execute = async () => {
        const authHeaders = await getTelegramAuthHeaders(initData);
        return fetchJSON<T>(input, {
            ...init,
            headers: mergeHeaders(authHeaders, init),
        });
    };

    try {
        return await execute();
    } catch (error) {
        if (error instanceof APIError && error.status === 401) {
            clearTelegramSession();
            return execute();
        }
        throw error;
    }
}
