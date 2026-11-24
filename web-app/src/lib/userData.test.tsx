import { describe, expect, it, vi } from 'vitest';
import { waitFor } from '@testing-library/react';
import { createUserDataStore, USER_DATA_TTL_MS } from './userData';
import { UserData } from './types';

const profile = (id: number, extra: Partial<UserData> = {}): UserData => ({
    user: { id, telegram_id: 42 },
    keys: [],
    is_active: false,
    expire_at: null,
    days_remaining: 0,
    trial_eligible: false,
    trial_days: 0,
    ...extra,
});

function stubFetch(payload: UserData | (() => UserData) = profile(1)) {
    const fetchMock = vi.fn(async () => {
        const body = typeof payload === 'function' ? payload() : payload;
        return new Response(JSON.stringify(body), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
        });
    });
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
}

const meCalls = (fetchMock: ReturnType<typeof stubFetch>) =>
    fetchMock.mock.calls.filter(([input]) => String(input) === '/api/me').length;

const meRequested = (fetchMock: ReturnType<typeof vi.fn>) =>
    fetchMock.mock.calls.some(([input]) => String(input) === '/api/me');

describe('user data store', () => {
    it('serves a second load from the store instead of refetching /api/me', async () => {
        const fetchMock = stubFetch(profile(7));
        let now = 1_000;
        const store = createUserDataStore(() => now);

        const first = await store.load('init-data');
        const second = await store.load('init-data');

        expect(first).toBe(second);
        expect(meCalls(fetchMock)).toBe(1);
    });

    it('refetches once the 60 s TTL elapses', async () => {
        const fetchMock = stubFetch(profile(7));
        let now = 1_000;
        const store = createUserDataStore(() => now);

        await store.load('init-data');
        now += USER_DATA_TTL_MS - 1;
        expect(store.peek('init-data')).not.toBeNull();

        await store.load('init-data');
        expect(meCalls(fetchMock)).toBe(1);

        now += 1;
        expect(store.peek('init-data')).toBeNull();
        await store.load('init-data');
        expect(meCalls(fetchMock)).toBe(2);
    });

    it('treats a forced load as a cache miss', async () => {
        const fetchMock = stubFetch(profile(7));
        const store = createUserDataStore(() => 1_000);

        await store.load('init-data');
        await store.load('init-data', undefined, { force: true });

        expect(meCalls(fetchMock)).toBe(2);
    });

    it('drops the stored profile on invalidate so a mutation is visible', async () => {
        let active = false;
        const fetchMock = stubFetch(() => profile(7, { is_active: active }));
        const store = createUserDataStore(() => 1_000);

        const before = await store.load('init-data');
        expect(before.is_active).toBe(false);

        active = true;
        store.invalidate();
        expect(store.peek('init-data')).toBeNull();

        const after = await store.load('init-data');
        expect(after.is_active).toBe(true);
        expect(meCalls(fetchMock)).toBe(2);
    });

    it('collapses concurrent loads into a single request', async () => {
        const fetchMock = stubFetch(profile(7));
        const store = createUserDataStore(() => 1_000);

        const [a, b] = await Promise.all([store.load('init-data'), store.load('init-data')]);

        expect(a).toBe(b);
        expect(meCalls(fetchMock)).toBe(1);
    });

    it('never serves one session the profile of another', async () => {
        const fetchMock = stubFetch(() => profile(9));
        const store = createUserDataStore(() => 1_000);

        await store.load('session-a');
        expect(store.peek('session-b')).toBeNull();
        await store.load('session-b');

        expect(meCalls(fetchMock)).toBe(2);
    });

    it('seeds the store from a profile it already produced', () => {
        const fetchMock = stubFetch();
        const store = createUserDataStore(() => 1_000);

        store.prime('init-data', profile(3));

        expect(store.peek('init-data')?.user.id).toBe(3);
        expect(meCalls(fetchMock)).toBe(0);
    });

    it('drops a response that was already on the wire when a mutation invalidated the store', async () => {
        let release: (value: UserData) => void = () => {};
        const fetchMock = vi.fn((input: RequestInfo | URL) => {
            if (String(input) === '/api/session') {
                return Promise.resolve(
                    new Response(JSON.stringify({ token: 'fresh-token', expires_at: '2999-01-01T00:00:00.000Z' }), {
                        status: 200,
                        headers: { 'Content-Type': 'application/json' },
                    }),
                );
            }
            return new Promise<Response>((resolve) => {
                release = (value) =>
                    resolve(
                        new Response(JSON.stringify(value), {
                            status: 200,
                            headers: { 'Content-Type': 'application/json' },
                        }),
                    );
            });
        });
        vi.stubGlobal('fetch', fetchMock);

        const store = createUserDataStore(() => 1_000);
        const pending = store.load('init-data');

        await waitFor(() => expect(meRequested(fetchMock)).toBe(true));
        store.invalidate();
        release(profile(7));
        await pending;

        // The in-flight answer describes the pre-mutation world, so it is not stored.
        expect(store.peek('init-data')).toBeNull();
    });

    it('rejects a load without a Mini App session instead of calling the API', async () => {
        const fetchMock = stubFetch();

        await expect(createUserDataStore().load('')).rejects.toThrow('missing initData');
        expect(meCalls(fetchMock)).toBe(0);
    });
});
