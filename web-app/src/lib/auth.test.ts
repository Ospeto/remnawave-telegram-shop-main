import { describe, expect, it, vi } from 'vitest';

import { clearTelegramSession, getTelegramAuthHeaders } from './auth';

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
});
