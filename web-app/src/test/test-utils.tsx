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

export function jsonResponse(data: unknown, status = 200): Response {
    return {
        ok: status >= 200 && status < 300,
        status,
        json: async () => data,
        text: async () => typeof data === 'string' ? data : JSON.stringify(data),
    } as Response;
}
