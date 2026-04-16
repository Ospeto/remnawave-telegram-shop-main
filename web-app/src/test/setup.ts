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

const sessionStorageMock = {
    getItem: (key: string) => storage.get(`session:${key}`) ?? null,
    setItem: (key: string, value: string) => {
        storage.set(`session:${key}`, String(value));
    },
    removeItem: (key: string) => {
        storage.delete(`session:${key}`);
    },
    clear: () => {
        for (const key of Array.from(storage.keys())) {
            if (key.startsWith('session:')) {
                storage.delete(key);
            }
        }
    },
    key: (index: number) => Array.from(storage.keys()).filter((key) => key.startsWith('session:'))[index]?.replace(/^session:/, '') ?? null,
    get length() {
        return Array.from(storage.keys()).filter((key) => key.startsWith('session:')).length;
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

Object.defineProperty(window, 'sessionStorage', {
    value: sessionStorageMock,
    configurable: true,
});

Object.defineProperty(globalThis, 'sessionStorage', {
    value: sessionStorageMock,
    configurable: true,
});

beforeEach(() => {
    localStorageMock.clear();
    sessionStorageMock.clear();
    localStorageMock.setItem('app_lang', 'en');
});

afterEach(() => {
    cleanup();
    localStorageMock.clear();
    sessionStorageMock.clear();
    vi.restoreAllMocks();
});
