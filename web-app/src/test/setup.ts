import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach, beforeEach, vi } from 'vitest';

const storage = new Map<string, string>();

const localStorageMock = {
    getItem: (key: string) => storage.get(key) ?? null,
    setItem: (key: string, value: string) => {
        storage.set(key, String(value));
    },
    removeItem: (key: string) => {
        storage.delete(key);
    },
    clear: () => {
        storage.clear();
    },
    key: (index: number) => Array.from(storage.keys())[index] ?? null,
    get length() {
        return storage.size;
    },
};

Object.defineProperty(window, 'localStorage', {
    value: localStorageMock,
    configurable: true,
});

Object.defineProperty(globalThis, 'localStorage', {
    value: localStorageMock,
    configurable: true,
});

beforeEach(() => {
    localStorageMock.clear();
    localStorageMock.setItem('app_lang', 'en');
});

afterEach(() => {
    cleanup();
    localStorageMock.clear();
    vi.restoreAllMocks();
});
