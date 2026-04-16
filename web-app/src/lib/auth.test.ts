import { describe, expect, it, vi } from 'vitest';

import { clearTelegramSession, fetchJSONWithTelegramAuth, getTelegramAuthHeaders } from './auth';

describe('getTelegramAuthHeaders', () => {
    it('exchanges initData once and reuses the stored bearer token', async () => {
        const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
            token: 'session-token-1',
            expires_at: new Date(Date.now() + 60_000).toISOString(),
        }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
        }));
        vi.stubGlobal('fetch', fetchMock);

        clearTelegramSession();

        const first = await getTelegramAuthHeaders('test-init-data');
        const second = await getTelegramAuthHeaders('test-init-data');

        expect(first.Authorization).toBe('Bearer session-token-1');
        expect(second.Authorization).toBe('Bearer session-token-1');
        expect(fetchMock).toHaveBeenCalledTimes(1);
        expect(fetchMock).toHaveBeenCalledWith('/api/session', expect.objectContaining({
            method: 'POST',
            headers: expect.objectContaining({
                Authorization: 'twa test-init-data',
            }),
        }));
    });

    it('deduplicates concurrent session exchanges for the same initData', async () => {
        let resolveFetch: ((response: Response) => void) | null = null;
        const fetchMock = vi.fn().mockImplementation(() => new Promise<Response>((resolve) => {
            resolveFetch = resolve;
        }));
        vi.stubGlobal('fetch', fetchMock);

        clearTelegramSession();

        const firstPromise = getTelegramAuthHeaders('test-init-data');
        const secondPromise = getTelegramAuthHeaders('test-init-data');

        expect(fetchMock).toHaveBeenCalledTimes(1);

        expect(resolveFetch).not.toBeNull();
        resolveFetch!(new Response(JSON.stringify({
            token: 'session-token-2',
            expires_at: new Date(Date.now() + 60_000).toISOString(),
        }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
        }));

        const [first, second] = await Promise.all([firstPromise, secondPromise]);
        expect(first.Authorization).toBe('Bearer session-token-2');
        expect(second.Authorization).toBe('Bearer session-token-2');
    });

    it('refreshes the stored bearer token from authenticated response headers', async () => {
        const fetchMock = vi.fn()
            .mockResolvedValueOnce(new Response(JSON.stringify({
                token: 'session-token-3',
                expires_at: new Date(Date.now() + 60_000).toISOString(),
            }), {
                status: 200,
                headers: { 'Content-Type': 'application/json' },
            }))
            .mockResolvedValueOnce(new Response(JSON.stringify({ ok: true }), {
                status: 200,
                headers: {
                    'Content-Type': 'application/json',
                    'X-Session-Token': 'session-token-4',
                    'X-Session-Expires-At': new Date(Date.now() + 120_000).toISOString(),
                },
            }));
        vi.stubGlobal('fetch', fetchMock);

        clearTelegramSession();

        await fetchJSONWithTelegramAuth('/api/me', 'test-init-data');
        const refreshed = await getTelegramAuthHeaders('test-init-data');

        expect(refreshed.Authorization).toBe('Bearer session-token-4');
    });
});
