import { createContext, useContext } from 'react';
import { fetchUserScopedJSONWithTelegramAuth } from './auth';
import { UserData } from './types';

/**
 * Shared `/api/me` store.
 *
 * Every page used to call `/api/me` cold on mount, so a customer walking
 * Home -> Plans -> Checkout -> Wallet paid four fan-out queries for one user
 * profile. The profile is now held in one store with a short TTL and shared
 * across route changes through a React context; any mutation that changes
 * wallet, keys or auto-renew invalidates it so the next screen reads fresh
 * state.
 */
export const USER_DATA_TTL_MS = 60_000;

export interface LoadUserDataOptions {
    /** Bypass the stored value (used after a mutation or an explicit retry). */
    force?: boolean;
}

export interface UserDataStore {
    /** Loads the profile, reusing the stored value while it is fresh. */
    load: (initData: string, telegramUserID?: number, options?: LoadUserDataOptions) => Promise<UserData>;
    /** Drops the stored profile after a mutation. */
    invalidate: () => void;
    /** Stores a profile the caller already knows to be current. */
    prime: (initData: string, data: UserData) => void;
    /** Reads the stored profile without triggering a request. */
    peek: (initData: string) => UserData | null;
}

interface StoredProfile {
    initData: string;
    data: UserData;
    fetchedAt: number;
}

/**
 * Builds an isolated store. The app uses one shared instance so route changes
 * hit the same entry; tests build their own so cases cannot leak into each
 * other.
 */
export function createUserDataStore(clock: () => number = () => Date.now()): UserDataStore {
    let stored: StoredProfile | null = null;
    let inFlight: { initData: string; promise: Promise<UserData> } | null = null;
    // Bumped by invalidate(): a response that was already on the wire when a mutation
    // dropped the profile carries pre-mutation state and must not be stored.
    let generation = 0;

    const peek = (initData: string): UserData | null => {
        if (!stored || !initData || stored.initData !== initData) return null;
        if (clock() - stored.fetchedAt >= USER_DATA_TTL_MS) return null;
        return stored.data;
    };

    const prime = (initData: string, data: UserData) => {
        stored = { initData, data, fetchedAt: clock() };
    };

    const invalidate = () => {
        stored = null;
        inFlight = null;
        generation += 1;
    };

    const load = async (
        initData: string,
        telegramUserID?: number,
        options: LoadUserDataOptions = {},
    ): Promise<UserData> => {
        if (!initData) {
            throw new Error('missing initData');
        }
        if (!options.force) {
            const cached = peek(initData);
            if (cached) return cached;
            if (inFlight && inFlight.initData === initData) {
                return inFlight.promise;
            }
        }

        const requestGeneration = generation;
        const request = fetchUserScopedJSONWithTelegramAuth<UserData>('/api/me', initData, telegramUserID)
            .then((data) => {
                if (generation === requestGeneration) {
                    prime(initData, data);
                }
                return data;
            })
            .finally(() => {
                if (inFlight?.promise === request) {
                    inFlight = null;
                }
            });

        inFlight = { initData, promise: request };
        return request;
    };

    return { load, invalidate, prime, peek };
}

/** The one store every route reads, so navigation reuses the profile. */
export const sharedUserDataStore = createUserDataStore();

export const UserDataStoreContext = createContext<UserDataStore>(sharedUserDataStore);

/** Provides a store (defaults to the shared one) to the route tree. */
export function UserDataProvider({
    store = sharedUserDataStore,
    children,
}: {
    store?: UserDataStore;
    children: React.ReactNode;
}) {
    return <UserDataStoreContext.Provider value={store}>{children}</UserDataStoreContext.Provider>;
}

/** The shared `/api/me` store used by the pages under the provider. */
export function useUserProfile(): UserDataStore {
    return useContext(UserDataStoreContext);
}

/** Drops the shared store's profile; the next load performs a real request. */
export function invalidateUserData() {
    sharedUserDataStore.invalidate();
}
