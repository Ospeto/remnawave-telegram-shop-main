# Admin Panel Hub Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Organize Mini App admin tools under one nested `/admin` hub with a single Home Admin entry, sectioned tool cards, and child pages that back to the hub.

**Architecture:** Nested React Router tree under `AdminLayout` (`Outlet` only). Index route renders `AdminHub` (admin-gated sectioned digital cards). Existing deep links `/admin/finance|plans|promos` stay valid as child routes. Home collapses three admin cards into one Admin card → `/admin`. Explicit path backs only (`/admin` or `/`); never `navigate(-1)`.

**Tech Stack:** React 18 + React Router 6, Vite, TypeScript, vitest + testing-library, existing Mini App digital-card CSS / Telegram BackButton / `fetchUserScopedJSONWithTelegramAuth`.

**Source of truth:** `docs/superpowers/specs/2026-07-12-admin-panel-hub-design.md`

## Global Constraints

- Mini App only — do **not** change Telegram bot `/admin` handlers or any Go backend.
- No new admin APIs; hub gates with existing `/api/me` only.
- Keep per-page `is_admin` checks on Finance/Plans/Promos (defense in depth; no shared auth context).
- Do not redesign Finance/Plans/Promos internals beyond back target + shell fit.
- Do not touch payment/wallet money paths.
- Back navigation uses explicit paths only (`navigate('/admin')` or `navigate('/')`), never `navigate(-1)`.
- Card UI must reuse Home `digital-card` pattern (icon, title, subtitle, chevron, press scale, `playClick`).
- i18n: add EN + MY keys for hub + Home Admin card; reuse existing `admin_*_card_*` keys for tool cards.
- Verification: `cd web-app && npm test` and `cd web-app && npm run build`.

---

## File Structure

| Path | Responsibility |
|------|----------------|
| `web-app/src/pages/AdminLayout.tsx` | **Create** — minimal full-height shell + `<Outlet />` |
| `web-app/src/pages/AdminHub.tsx` | **Create** — hub gate, section header, three tool cards, BackButton → `/` |
| `web-app/src/pages/AdminHub.test.tsx` | **Create** — admin sees tools; non-admin forbidden; back registers `/` |
| `web-app/src/App.tsx` | Nested `/admin` route tree |
| `web-app/src/pages/Home.tsx` | Replace three admin cards with one Admin card → `/admin` |
| `web-app/src/pages/AdminFinance.tsx` | BackButton handler → `/admin` |
| `web-app/src/pages/AdminPlans.tsx` | `handleBack` → `/admin` |
| `web-app/src/pages/AdminPromos.tsx` | `handleBack` → `/admin` |
| `web-app/src/lib/translations.ts` | Hub + Home Admin card keys EN/MY |
| `web-app/src/pages/Home.test.tsx` | Single Admin link; no Finance/Plans/Promos cards on Home |
| `web-app/src/App.routes.test.tsx` | Nested route registration contract |
| `web-app/src/pages/AdminFinance.test.tsx` | Assert BackButton navigates to `/admin` |
| `web-app/src/pages/AdminPlans.test.tsx` | Assert BackButton navigates to `/admin` |
| `web-app/src/pages/AdminPromos.test.tsx` | Assert BackButton navigates to `/admin` |

**Explicitly not modified:** any `internal/**` Go code, Telegram handlers, migrations, payment/wallet packages.

---

## Shared interface contracts (Consumes / Produces)

```tsx
// AdminLayout — no props
export function AdminLayout(): JSX.Element
// Renders: <div className="admin-layout"> <Outlet /> </div>

// AdminHub — no props
export function AdminHub(): JSX.Element
// Gate: fetchUserScopedJSONWithTelegramAuth<UserData>('/api/me', ...)
// Back: navigate('/')
// Links: /admin/finance, /admin/plans, /admin/promos

// Route tree in App.tsx (exact):
// <Route path="/admin" element={<AdminLayout />}>
//   <Route index element={<AdminHub />} />
//   <Route path="finance" element={<AdminFinance />} />
//   <Route path="plans" element={<AdminPlans />} />
//   <Route path="promos" element={<AdminPromos />} />
// </Route>

// i18n keys (both en and my):
// admin_hub_title, admin_hub_subtitle, admin_hub_section_shop,
// admin_hub_forbidden, admin_hub_loading,
// admin_card_title, admin_card_subtitle
// Tool cards reuse: admin_finance_card_*, admin_plans_card_*, admin_promos_card_*
```

---

### Task 1: i18n keys (EN + MY)

**Files:**
- Modify: `web-app/src/lib/translations.ts` (EN block ends ~L326 before `my:`; MY block ends ~L651)

**Interfaces:**
- Consumes: existing `translations` Record shape
- Produces: seven new keys in both `en` and `my`

- [ ] **Step 1: Add English keys**

Insert immediately before the closing of the `en` object (before `finance_admin_required` is fine, or after it — keep alphabetical-ish grouping with other admin keys). Recommended placement: after `admin_finance_card_subtitle` / near other admin card keys, and ensure both languages get all keys.

Add to `en`:

```ts
'admin_hub_title': 'Admin',
'admin_hub_subtitle': 'Manage shop tools',
'admin_hub_section_shop': 'Shop',
'admin_hub_forbidden': 'Admin access required.',
'admin_hub_loading': 'Loading…',
'admin_card_title': 'Admin',
'admin_card_subtitle': 'Finance, plans, and promos',
```

- [ ] **Step 2: Add Myanmar keys**

Add to `my` (grounded tone matching existing admin cards):

```ts
'admin_hub_title': 'Admin',
'admin_hub_subtitle': 'ဆိုင်ကိရိယာများကို စီမံရန်',
'admin_hub_section_shop': 'ဆိုင်',
'admin_hub_forbidden': 'Admin access လိုအပ်ပါသည်။',
'admin_hub_loading': 'ခေတ္တစောင့်ဆိုင်းပါ...',
'admin_card_title': 'Admin',
'admin_card_subtitle': 'ဘဏ္ဍာရေး၊ plan နှင့် promo များ',
```

- [ ] **Step 3: Commit**

```bash
git add web-app/src/lib/translations.ts
git commit -m "feat(web-app): add admin hub i18n keys EN+MY"
```

---

### Task 2: AdminLayout shell

**Files:**
- Create: `web-app/src/pages/AdminLayout.tsx`

**Interfaces:**
- Consumes: `react-router-dom` `Outlet`
- Produces: `export function AdminLayout()`

- [ ] **Step 1: Create AdminLayout**

```tsx
import { Outlet } from 'react-router-dom';

export function AdminLayout() {
    return (
        <div
            className="admin-layout"
            style={{
                minHeight: '100%',
                width: '100%',
            }}
        >
            <Outlet />
        </div>
    );
}
```

No side nav, no tabs, no data fetching. Children keep their own headers.

- [ ] **Step 2: Commit**

```bash
git add web-app/src/pages/AdminLayout.tsx
git commit -m "feat(web-app): add AdminLayout outlet shell"
```

---

### Task 3: AdminHub page + tests (TDD)

**Files:**
- Create: `web-app/src/pages/AdminHub.test.tsx`
- Create: `web-app/src/pages/AdminHub.tsx`
- Test: `web-app/src/pages/AdminHub.test.tsx`

**Interfaces:**
- Consumes: `AdminLayout` optional for mount; `/api/me`; translations keys from Task 1; Home digital-card pattern
- Produces: gated hub with three tool links + BackButton → `/`

- [ ] **Step 1: Write failing AdminHub tests**

Create `web-app/src/pages/AdminHub.test.tsx`:

```tsx
import { screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { AdminHub } from './AdminHub';
import { jsonResponse, renderWithAppProviders, seedTelegramSession } from '../test/test-utils';

const telegramState = vi.hoisted(() => ({
    tg: {
        BackButton: {
            show: vi.fn(),
            hide: vi.fn(),
            onClick: vi.fn(),
            offClick: vi.fn(),
        },
        openLink: vi.fn(),
        initDataUnsafe: { user: { id: 42 } },
    },
    initData: 'test-init-data',
    user: { id: 42 },
    close: vi.fn(),
    openLink: vi.fn(),
    colorScheme: 'light',
    themeParams: {},
}));

vi.mock('../lib/twa', () => ({
    useTelegram: () => telegramState,
}));

vi.mock('../lib/useMXBrownSound', () => ({
    useMXBrownSound: () => ({ playClick: vi.fn() }),
}));

describe('AdminHub', () => {
    const fetchMock = vi.fn();

    beforeEach(() => {
        fetchMock.mockReset();
        telegramState.tg.BackButton.show.mockReset();
        telegramState.tg.BackButton.onClick.mockReset();
        telegramState.tg.BackButton.offClick.mockReset();
        vi.stubGlobal('fetch', fetchMock);
        seedTelegramSession();
    });

    it('blocks non-admin users from the hub', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/me') {
                return jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [],
                    is_active: false,
                    expire_at: null,
                    days_remaining: 0,
                    trial_eligible: false,
                    trial_days: 0,
                    is_admin: false,
                });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin', element: <AdminHub /> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin']);

        expect(await screen.findByRole('alert')).toHaveTextContent('Admin access required.');
        expect(screen.queryByRole('link', { name: /Finance/i })).toBeNull();
        expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    it('shows Shop section and three tool links for admins', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/me') {
                return jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [],
                    is_active: false,
                    expire_at: null,
                    days_remaining: 0,
                    trial_eligible: false,
                    trial_days: 0,
                    is_admin: true,
                });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin', element: <AdminHub /> },
            { path: '/admin/finance', element: <div>Finance</div> },
            { path: '/admin/plans', element: <div>Plans</div> },
            { path: '/admin/promos', element: <div>Promos</div> },
            { path: '/', element: <div>Home</div> },
        ], ['/admin']);

        expect(await screen.findByRole('heading', { name: 'Admin' })).toBeTruthy();
        expect(screen.getByText('Shop')).toBeTruthy();
        expect(screen.getByRole('link', { name: /Finance/i })).toHaveAttribute('href', '/admin/finance');
        expect(screen.getByRole('link', { name: /Plans/i })).toHaveAttribute('href', '/admin/plans');
        expect(screen.getByRole('link', { name: /Promo Codes/i })).toHaveAttribute('href', '/admin/promos');
    });

    it('registers Telegram BackButton to navigate home', async () => {
        fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
            const url = String(input);
            if (url === '/api/me') {
                return jsonResponse({
                    user: { id: 1, telegram_id: 42 },
                    keys: [],
                    is_active: false,
                    expire_at: null,
                    days_remaining: 0,
                    trial_eligible: false,
                    trial_days: 0,
                    is_admin: true,
                });
            }
            throw new Error(`Unhandled fetch: ${url}`);
        });

        renderWithAppProviders([
            { path: '/admin', element: <AdminHub /> },
            { path: '/', element: <div>Home Page</div> },
        ], ['/admin']);

        await screen.findByRole('heading', { name: 'Admin' });

        expect(telegramState.tg.BackButton.show).toHaveBeenCalled();
        expect(telegramState.tg.BackButton.onClick).toHaveBeenCalled();

        const handler = telegramState.tg.BackButton.onClick.mock.calls[0][0] as () => void;
        handler();

        await waitFor(() => {
            expect(screen.getByText('Home Page')).toBeTruthy();
        });
    });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd web-app && npm test -- src/pages/AdminHub.test.tsx
```

Expected: FAIL — `AdminHub` module not found / cannot resolve.

- [ ] **Step 3: Implement AdminHub**

Create `web-app/src/pages/AdminHub.tsx` following AdminPromos gate pattern + Home digital-card markup:

```tsx
import { useCallback, useEffect, useState, type CSSProperties } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { clearTelegramSession, fetchUserScopedJSONWithTelegramAuth } from '../lib/auth';
import { isAPIStatus } from '../lib/http';
import { useLanguage } from '../lib/LanguageContext';
import { UserData } from '../lib/types';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { useTelegram } from '../lib/twa';

type ToolCard = {
    to: string;
    titleKey: string;
    subtitleKey: string;
    icon: JSX.Element;
};

export function AdminHub() {
    const { tg, initData, close } = useTelegram();
    const { t } = useLanguage();
    const navigate = useNavigate();
    const { playClick } = useMXBrownSound();
    const [loading, setLoading] = useState(true);
    const [authExpired, setAuthExpired] = useState(false);
    const [accessDenied, setAccessDenied] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const handleBack = useCallback(() => {
        navigate('/');
    }, [navigate]);

    const load = useCallback(async () => {
        if (!initData) {
            setLoading(false);
            return;
        }

        setLoading(true);
        setError(null);
        setAuthExpired(false);
        setAccessDenied(false);

        try {
            const meData = await fetchUserScopedJSONWithTelegramAuth<UserData>(
                '/api/me',
                initData,
                tg?.initDataUnsafe?.user?.id,
            );
            if (!meData.is_admin) {
                setAccessDenied(true);
                return;
            }
        } catch (err) {
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (isAPIStatus(err, 403)) {
                setAccessDenied(true);
                return;
            }
            setError(err instanceof Error ? err.message : t('admin_hub_forbidden'));
        } finally {
            setLoading(false);
        }
    }, [initData, t, tg]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [handleBack, tg]);

    useEffect(() => {
        void load();
    }, [load]);

    if (loading) return <LoadingScreen message={t('admin_hub_loading')} />;
    if (authExpired) {
        return (
            <SessionExpiredScreen
                title={t('session_expired_title')}
                message={t('session_expired_desc')}
                reloadLabel={t('session_expired_reload')}
                closeLabel={t('session_expired_close')}
                onReload={() => { window.location.reload(); }}
                onClose={() => { close(); }}
            />
        );
    }
    if (!initData) {
        return (
            <div className="screen-center">
                <div style={{ fontSize: 48 }}>📱</div>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Wavy Private Server Shop</h1>
                <p className="text-hint" style={{ margin: 0 }}>{t('open_in_tg')}</p>
            </div>
        );
    }
    if (accessDenied) {
        return <ErrorScreen message={t('admin_hub_forbidden')} />;
    }
    if (error) {
        return (
            <ErrorScreen
                message={error}
                onRetry={() => { void load(); }}
                retryLabel={t('retry')}
            />
        );
    }

    const tools: ToolCard[] = [
        {
            to: '/admin/finance',
            titleKey: 'admin_finance_card_title',
            subtitleKey: 'admin_finance_card_subtitle',
            icon: (
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M4 19V5" />
                    <path d="M4 19h16" />
                    <path d="M8 15l3-4 3 2 4-6" />
                </svg>
            ),
        },
        {
            to: '/admin/plans',
            titleKey: 'admin_plans_card_title',
            subtitleKey: 'admin_plans_card_subtitle',
            icon: (
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M4 6h16" />
                    <path d="M4 12h16" />
                    <path d="M4 18h16" />
                    <path d="M8 3v18" />
                </svg>
            ),
        },
        {
            to: '/admin/promos',
            titleKey: 'admin_promos_card_title',
            subtitleKey: 'admin_promos_card_subtitle',
            icon: (
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M4 7h16" />
                    <path d="M7 12h10" />
                    <path d="M9 17h6" />
                </svg>
            ),
        },
    ];

    const cardStyle: CSSProperties = {
        padding: '16px 20px',
        display: 'flex',
        alignItems: 'center',
        gap: 14,
        textDecoration: 'none',
        color: 'var(--digital-card-text)',
        transition: 'transform 0.15s cubic-bezier(0.34, 1.56, 0.64, 1)',
        cursor: 'pointer',
    };

    return (
        <div className="animate-fade-in" style={{ padding: '20px 16px 32px', display: 'grid', gap: 16 }}>
            <header style={{ display: 'grid', gap: 4 }}>
                <h1 style={{ margin: 0, fontSize: 'var(--font-h2)', fontWeight: 'var(--weight-bold)' }}>
                    {t('admin_hub_title')}
                </h1>
                <p className="text-hint" style={{ margin: 0, fontSize: 'var(--font-caption)' }}>
                    {t('admin_hub_subtitle')}
                </p>
            </header>

            <section style={{ display: 'grid', gap: 10 }}>
                <h2 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: 'var(--digital-card-hint)', letterSpacing: '0.4px', textTransform: 'uppercase' }}>
                    {t('admin_hub_section_shop')}
                </h2>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 10 }}>
                    {tools.map((tool) => (
                        <Link
                            key={tool.to}
                            to={tool.to}
                            className="digital-card animate-slide-up"
                            style={cardStyle}
                            onClick={() => playClick()}
                            onMouseDown={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
                            onMouseUp={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
                            onTouchStart={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
                            onTouchEnd={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
                            onMouseLeave={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
                        >
                            <div style={{
                                width: 44, height: 44, borderRadius: 12,
                                background: 'var(--digital-card-inner-bg)',
                                backdropFilter: 'blur(10px)',
                                display: 'flex', alignItems: 'center', justifyContent: 'center',
                                color: 'var(--digital-card-text)',
                                boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
                            }} aria-hidden="true">
                                {tool.icon}
                            </div>
                            <div style={{ flex: 1 }}>
                                <div style={{ fontWeight: 'var(--weight-bold)', fontSize: '15px', color: 'var(--digital-card-text)', letterSpacing: '0.2px' }}>
                                    {t(tool.titleKey)}
                                </div>
                                <div style={{ fontSize: '13px', color: 'var(--digital-card-hint)', marginTop: 1 }}>
                                    {t(tool.subtitleKey)}
                                </div>
                            </div>
                            <div style={{
                                width: 28, height: 28, borderRadius: 14,
                                background: 'var(--digital-card-inner-bg)',
                                display: 'flex', alignItems: 'center', justifyContent: 'center',
                                fontSize: 14, color: 'var(--digital-card-text)',
                            }} aria-hidden="true">→</div>
                        </Link>
                    ))}
                </div>
            </section>
        </div>
    );
}
```

Notes for implementer:
- Use `CSSProperties` from `react` (not `React.CSSProperties` without a React namespace import).
- Do not fetch promos/plans/finance data on the hub.

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd web-app && npm test -- src/pages/AdminHub.test.tsx
```

Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add web-app/src/pages/AdminHub.tsx web-app/src/pages/AdminHub.test.tsx
git commit -m "feat(web-app): add AdminHub with gate and shop tool cards"
```

---

### Task 4: Nested routes in App.tsx + route contract test

**Files:**
- Modify: `web-app/src/App.tsx`
- Modify: `web-app/src/App.routes.test.tsx`

**Interfaces:**
- Consumes: `AdminLayout`, `AdminHub`, existing admin pages
- Produces: nested `/admin` tree; deep links unchanged

- [ ] **Step 1: Update App.routes.test.tsx to read App.tsx source (failing until routes nest)**

Replace `web-app/src/App.routes.test.tsx` with:

```tsx
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Assert nested admin registration against real App.tsx source.
const appSource = readFileSync(resolve(__dirname, './App.tsx'), 'utf8');

const REQUIRED = [
  "import { AdminLayout } from './pages/AdminLayout'",
  "import { AdminHub } from './pages/AdminHub'",
  "import { AdminFinance } from './pages/AdminFinance'",
  "import { AdminPlans } from './pages/AdminPlans'",
  "import { AdminPromos } from './pages/AdminPromos'",
  '<Route path="/admin" element={<AdminLayout />}>',
  '<Route index element={<AdminHub />} />',
  '<Route path="finance" element={<AdminFinance />} />',
  '<Route path="plans" element={<AdminPlans />} />',
  '<Route path="promos" element={<AdminPromos />} />',
] as const;

describe('App admin route registration', () => {
  it('registers nested /admin layout + hub + children in App.tsx', () => {
    for (const snippet of REQUIRED) {
      expect(appSource).toContain(snippet);
    }
  });
});
```

- [ ] **Step 2: Nest routes in App.tsx**

Replace flat admin routes with:

```tsx
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Home } from './pages/Home';
import { Plans } from './pages/Plans';
import { Checkout } from './pages/Checkout';
import { Wallet } from './pages/Wallet';
import { AdminLayout } from './pages/AdminLayout';
import { AdminHub } from './pages/AdminHub';
import { AdminPromos } from './pages/AdminPromos';
import { AdminPlans } from './pages/AdminPlans';
import { AdminFinance } from './pages/AdminFinance';

import { ThemeProvider } from './lib/ThemeProvider';

function App() {
    return (
        <ThemeProvider>
            <BrowserRouter>
                <Routes>
                    <Route path="/" element={<Home />} />
                    <Route path="/admin" element={<AdminLayout />}>
                        <Route index element={<AdminHub />} />
                        <Route path="finance" element={<AdminFinance />} />
                        <Route path="plans" element={<AdminPlans />} />
                        <Route path="promos" element={<AdminPromos />} />
                    </Route>
                    <Route path="/plans" element={<Plans />} />
                    <Route path="/wallet" element={<Wallet />} />
                    <Route path="/checkout" element={<Checkout />} />
                    <Route path="/checkout/:planIndex" element={<Checkout />} />
                </Routes>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
```

- [ ] **Step 3: Run route contract test**

```bash
cd web-app && npm test -- src/App.routes.test.tsx
```

Expected: PASS (each `REQUIRED` snippet present in `App.tsx`).

- [ ] **Step 4: Commit**

```bash
git add web-app/src/App.tsx web-app/src/App.routes.test.tsx
git commit -m "feat(web-app): nest admin routes under AdminLayout hub"
```

---

### Task 5: Child pages back → `/admin` + tests

**Files:**
- Modify: `web-app/src/pages/AdminFinance.tsx` (~L191)
- Modify: `web-app/src/pages/AdminPlans.tsx` (~L59–61)
- Modify: `web-app/src/pages/AdminPromos.tsx` (~L43–45)
- Modify: `web-app/src/pages/AdminFinance.test.tsx`
- Modify: `web-app/src/pages/AdminPlans.test.tsx`
- Modify: `web-app/src/pages/AdminPromos.test.tsx`

**Interfaces:**
- Consumes: existing BackButton wiring
- Produces: all three children navigate to `/admin` on back

- [ ] **Step 1: Change back targets (three one-line edits)**

`AdminPromos.tsx`:

```tsx
const handleBack = useCallback(() => {
    navigate('/admin');
}, [navigate]);
```

`AdminPlans.tsx`:

```tsx
const handleBack = useCallback(() => {
    navigate('/admin');
}, [navigate]);
```

`AdminFinance.tsx` (inside the BackButton effect):

```tsx
const handler = () => navigate('/admin');
```

Do not change hide/show/offClick behavior on Finance.

- [ ] **Step 1b: Reset BackButton mocks in each child test `beforeEach`**

In `AdminPromos.test.tsx`, `AdminPlans.test.tsx`, and `AdminFinance.test.tsx` `beforeEach`, add:

```ts
telegramState.tg.BackButton.show.mockReset();
telegramState.tg.BackButton.hide.mockReset();
telegramState.tg.BackButton.onClick.mockReset();
telegramState.tg.BackButton.offClick.mockReset();
```

Without this, `onClick.mock.calls[0]` can pick a handler from an earlier test (still `navigate('/')`) and flake.

- [ ] **Step 2: Add back-navigation tests**

For each of `AdminPromos.test.tsx`, `AdminPlans.test.tsx`, `AdminFinance.test.tsx`, add a test that:
1. Mocks `/api/me` as admin (and any required follow-up fetches so the page finishes loading — Finance needs a revenue report mock; Promos needs promos list; Plans needs plans list — copy existing happy-path mocks from the same file).
2. Renders with routes including `{ path: '/admin', element: <div>Admin Hub</div> }` and the page path.
3. After load, takes the **latest** BackButton handler and invokes it.
4. Asserts `screen.getByText('Admin Hub')` appears.

Example for AdminPromos (adapt paths/mocks per file):

```tsx
it('navigates to /admin on Telegram BackButton', async () => {
    fetchMock.mockImplementation(async (input: RequestInfo | URL) => {
        const url = String(input);
        if (url === '/api/me') {
            return jsonResponse({
                user: { id: 1, telegram_id: 42 },
                keys: [],
                is_active: false,
                expire_at: null,
                days_remaining: 0,
                trial_eligible: false,
                trial_days: 0,
                is_admin: true,
            });
        }
        if (url === '/api/admin/promos') {
            return jsonResponse([]);
        }
        throw new Error(`Unhandled fetch: ${url}`);
    });

    renderWithAppProviders([
        { path: '/admin/promos', element: <AdminPromos /> },
        { path: '/admin', element: <div>Admin Hub</div> },
    ], ['/admin/promos']);

    await waitFor(() => {
        expect(telegramState.tg.BackButton.onClick).toHaveBeenCalled();
    });

    const calls = telegramState.tg.BackButton.onClick.mock.calls;
    const handler = calls[calls.length - 1][0] as () => void;
    handler();

    expect(await screen.findByText('Admin Hub')).toBeTruthy();
});
```

For AdminPlans: mock `/api/admin/plans` with `[]` (or existing sample).  
For AdminFinance: BackButton effect is mount-only (`[navigate, tg]`) — still mock `/api/me` + revenue so the page does not error-noise; then invoke latest handler and assert hub.

- [ ] **Step 3: Run focused tests**

```bash
cd web-app && npm test -- src/pages/AdminPromos.test.tsx src/pages/AdminPlans.test.tsx src/pages/AdminFinance.test.tsx
```

Expected: PASS (including new back tests; existing tests still pass with flat mounts).

- [ ] **Step 4: Commit**

```bash
git add web-app/src/pages/AdminFinance.tsx web-app/src/pages/AdminPlans.tsx web-app/src/pages/AdminPromos.tsx \
  web-app/src/pages/AdminFinance.test.tsx web-app/src/pages/AdminPlans.test.tsx web-app/src/pages/AdminPromos.test.tsx
git commit -m "feat(web-app): point admin child BackButton to /admin hub"
```

---

### Task 6: Home single Admin card + Home tests

**Files:**
- Modify: `web-app/src/pages/Home.tsx` (~L265–402)
- Modify: `web-app/src/pages/Home.test.tsx` (~L91–191)

**Interfaces:**
- Consumes: `admin_card_title`, `admin_card_subtitle`
- Produces: one Admin link to `/admin` when `is_admin`

- [ ] **Step 1: Update Home tests first**

Replace the four admin-related tests (`shows an admin promo card…`, `hides the admin promo card…`, `shows Finance admin card…`, `hides Finance admin card…`) with two clearer tests:

```tsx
it('shows a single Admin card linking to /admin for admins', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
        user: { id: 1, telegram_id: 42 },
        keys: [],
        is_active: false,
        expire_at: null,
        days_remaining: 0,
        trial_eligible: false,
        trial_days: 0,
        is_admin: true,
    }));

    renderWithAppProviders([
        { path: '/', element: <Home /> },
        { path: '/wallet', element: <div>Wallet</div> },
        { path: '/admin', element: <div>Admin Hub</div> },
    ], ['/']);

    expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
    const adminLink = screen.getByRole('link', { name: /Admin Finance, plans, and promos/i });
    expect(adminLink).toHaveAttribute('href', '/admin');
    expect(screen.queryByRole('link', { name: /Promo Codes/i })).toBeNull();
    expect(screen.queryByRole('link', { name: /Plans Add, edit, and archive plan pricing/i })).toBeNull();
    // Finance tool card title must not appear as its own Home link
    expect(screen.queryByRole('link', { name: /^Finance Income/i })).toBeNull();
});

it('hides the Admin card for non-admin users', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
        user: { id: 1, telegram_id: 42 },
        keys: [],
        is_active: false,
        expire_at: null,
        days_remaining: 0,
        trial_eligible: false,
        trial_days: 0,
        is_admin: false,
    }));

    renderWithAppProviders([
        { path: '/', element: <Home /> },
        { path: '/wallet', element: <div>Wallet</div> },
        { path: '/admin', element: <div>Admin Hub</div> },
    ], ['/']);

    expect(await screen.findByRole('heading', { name: 'Wavy Private Server' })).toBeTruthy();
    expect(screen.queryByRole('link', { name: /Admin Finance, plans, and promos/i })).toBeNull();
    expect(screen.queryByRole('link', { name: /Promo Codes/i })).toBeNull();
    expect(screen.queryByRole('link', { name: /Plans Add, edit, and archive plan pricing/i })).toBeNull();
});
```

Remove the obsolete separate Finance card tests (covered by the two above).

- [ ] **Step 2: Run Home tests — expect fail**

```bash
cd web-app && npm test -- src/pages/Home.test.tsx
```

Expected: FAIL — still three cards / wrong link text.

- [ ] **Step 3: Replace three admin cards with one**

In `Home.tsx`, delete the three `{data?.is_admin && ( <Link to="/admin/finance|plans|promos" ...> )}` blocks (lines ~265–402) and replace with a single card:

```tsx
{data?.is_admin && (
    <Link
        to="/admin"
        className="digital-card animate-slide-up"
        style={{
            padding: '16px 20px', display: 'flex', alignItems: 'center', gap: 14,
            textDecoration: 'none', color: 'var(--digital-card-text)',
            transition: 'transform 0.15s cubic-bezier(0.34, 1.56, 0.64, 1)',
            cursor: 'pointer',
        }}
        onClick={() => playClick()}
        onMouseDown={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
        onMouseUp={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
        onTouchStart={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
        onTouchEnd={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
        onMouseLeave={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
    >
        <div style={{
            width: 44, height: 44, borderRadius: 12,
            background: 'var(--digital-card-inner-bg)',
            backdropFilter: 'blur(10px)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            color: 'var(--digital-card-text)',
            boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
        }} aria-hidden="true">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <path d="M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3z" />
                <path d="M12 12l8-4.5" />
                <path d="M12 12v9" />
                <path d="M12 12L4 7.5" />
            </svg>
        </div>
        <div style={{ flex: 1 }}>
            <div style={{ fontWeight: 'var(--weight-bold)', fontSize: '15px', color: 'var(--digital-card-text)', letterSpacing: '0.2px' }}>
                {t('admin_card_title')}
            </div>
            <div style={{ fontSize: '13px', color: 'var(--digital-card-hint)', marginTop: 1 }}>
                {t('admin_card_subtitle')}
            </div>
        </div>
        <div style={{
            width: 28, height: 28, borderRadius: 14,
            background: 'var(--digital-card-inner-bg)',
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            fontSize: 14, color: 'var(--digital-card-text)',
        }} aria-hidden="true">→</div>
    </Link>
)}
```

- [ ] **Step 4: Run Home tests**

```bash
cd web-app && npm test -- src/pages/Home.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web-app/src/pages/Home.tsx web-app/src/pages/Home.test.tsx
git commit -m "feat(web-app): single Home Admin card to /admin hub"
```

---

### Task 7: Full frontend verification

**Files:** none new — verify only

- [ ] **Step 1: Run full frontend test suite**

```bash
cd web-app && npm test
```

Expected: all tests PASS.

- [ ] **Step 2: Production build**

```bash
cd web-app && npm run build
```

Expected: `tsc && vite build` succeeds with no type errors.

- [ ] **Step 3: Manual smoke checklist (if Mini App available)**

1. Admin Home shows one Admin card (not three).
2. Tap Admin → hub with Shop section + Finance / Plans / Promos.
3. Open each tool; Telegram BackButton returns to hub.
4. Hub BackButton returns to customer Home.
5. Non-admin: no Admin card; direct `/admin` shows forbidden.

- [ ] **Step 4: Final commit only if any verify fixes were needed**

```bash
# only if fixes landed
git add -A web-app/
git commit -m "fix(web-app): admin hub verify follow-ups"
```

---

## Spec coverage checklist

| Spec requirement | Task |
|------------------|------|
| Nested `/admin` shell + hub index | 2, 3, 4 |
| Children finance/plans/promos under layout; URL stability | 4 |
| Home single Admin card → `/admin` | 6 |
| Children back → `/admin`; hub → `/` | 3, 5 |
| Sectioned Shop cards reusing card title/subtitle keys | 3 |
| New i18n EN+MY | 1 |
| Per-page is_admin retained; hub gate | 3, 5 (no removal) |
| No backend / Telegram bot changes | Global constraints |
| Home / App.routes / AdminHub / child back tests | 3, 4, 5, 6 |
| `npm test` + `npm run build` | 7 |

## Placeholder / consistency self-review

- No TBD/TODO left in tasks.
- Back path is always `'/admin'` for children and `'/'` for hub.
- Route snippets in Task 4 match App.tsx code block exactly.
- i18n keys in Task 1 match AdminHub/Home usage in Tasks 3 and 6.
- Child page feature logic intentionally untouched beyond back target.

## Execution handoff

Plan complete and saved to `docs/superpowers/plans/2026-07-12-admin-panel-hub.md`.

**Two execution options:**

1. **Subagent-Driven (recommended)** — fresh subagent per task, review between tasks  
2. **Inline Execution** — execute tasks in this session with checkpoints  

Deepwork continues with oracle plan review before implementation starts.
