import { ReactElement } from 'react';
import { render } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { LanguageProvider } from '../lib/LanguageContext';

interface RouteConfig {
    path: string;
    element: ReactElement;
}

export function renderWithAppProviders(routes: RouteConfig[], initialEntries: string[]) {
    localStorage.setItem('app_lang', 'en');

    return render(
        <LanguageProvider>
            <MemoryRouter initialEntries={initialEntries}>
                <Routes>
                    {routes.map((route) => (
                        <Route key={route.path} path={route.path} element={route.element} />
                    ))}
                </Routes>
            </MemoryRouter>
        </LanguageProvider>,
    );
}

export function jsonResponse(data: unknown, status = 200, headersInit?: HeadersInit): Response {
    const headers = new Headers(headersInit);
    if (!headers.has('Content-Type')) {
        headers.set('Content-Type', 'application/json');
    }
    return {
        ok: status >= 200 && status < 300,
        status,
        headers,
        json: async () => data,
        text: async () => typeof data === 'string' ? data : JSON.stringify(data),
    } as Response;
}

export function seedTelegramSession(
    token = 'session-token',
    expiresAt = '2999-01-01T00:00:00.000Z',
    initData = 'test-init-data',
) {
    sessionStorage.setItem('telegram_api_session_v1', JSON.stringify({
        token,
        expiresAt,
        initData,
    }));
}
