# Admin Panel Hub — Design

**Date:** 2026-07-12  
**Status:** Approved for implementation planning  
**Scope:** Mini App only (Telegram bot `/admin` unchanged)

## Problem

Mini App admin tools are scattered on customer Home as three separate cards (Finance, Plans, Promos). There is no `/admin` hub, no shared admin shell, and each admin page backs out to customer Home. Admins cannot open “admin” as one place.

## Goals

- One Admin entry on Home → `/admin` hub.
- Sectioned hub cards grouping admin tools.
- Nested `/admin` shell with consistent chrome and navigation.
- Child pages (Finance / Plans / Promos) back to the hub; hub backs to customer Home.
- Preserve existing admin page behavior, APIs, and auth gates.
- Update i18n (EN + MY) and frontend tests for the new navigation.

## Non-goals

- Telegram bot `/admin` dashboard or any bot handler changes
- Backend API renames or new admin endpoints
- Moving Telegram ops (payments, backups, sync, test mode, E2E) into Mini App
- Config-driven admin registry / plugin system
- Redesigning Finance / Plans / Promos page internals beyond back navigation and shell fit
- Deep-link migration campaign beyond keeping existing `/admin/*` paths working

## Decisions (locked)

| Topic | Choice |
|-------|--------|
| Scope | Mini App only |
| Hub layout | Sectioned cards on `/admin` |
| Home entry | Single Admin card → `/admin` (replace three cards) |
| Back navigation | Children → `/admin`; hub → `/` |
| Architecture | Nested `/admin` shell + hub index |

## Current state (baseline)

- Routes (flat): `/admin/promos`, `/admin/plans`, `/admin/finance` in `web-app/src/App.tsx`
- Home (`web-app/src/pages/Home.tsx` ~265–402): three `is_admin` digital-card links
- Pages: `AdminFinance.tsx`, `AdminPlans.tsx`, `AdminPromos.tsx` — each checks `is_admin` via `/api/me`; Telegram `BackButton` / in-page back uses `navigate('/')`
- No shared admin layout component
- Tests: `Home.test.tsx` expects three admin links; `App.routes.test.tsx` asserts finance route registration; per-page admin tests mount flat routes

## Architecture

### Route tree

```text
/admin                    → AdminLayout + AdminHub (index)
/admin/finance            → AdminLayout + AdminFinance
/admin/plans              → AdminLayout + AdminPlans
/admin/promos             → AdminLayout + AdminPromos
```

React Router nested routes:

```tsx
<Route path="/admin" element={<AdminLayout />}>
  <Route index element={<AdminHub />} />
  <Route path="finance" element={<AdminFinance />} />
  <Route path="plans" element={<AdminPlans />} />
  <Route path="promos" element={<AdminPromos />} />
</Route>
```

**URL stability:** Existing deep links `/admin/finance`, `/admin/plans`, `/admin/promos` remain valid (same paths, nested under layout).

### Components

| Unit | Responsibility |
|------|----------------|
| `AdminLayout` | Shared admin chrome; renders `<Outlet />`; optional shared page frame (padding/container) matching existing admin pages; does **not** re-fetch admin data for children |
| `AdminHub` | Sectioned card grid; admin gate; BackButton → `/` |
| `AdminFinance` / `AdminPlans` / `AdminPromos` | Unchanged feature logic; back target becomes `/admin` |
| Home | Single Admin digital-card when `is_admin` |

### Auth / access

- **Home:** show Admin card only when `data?.is_admin` (same as today).
- **Hub:** gate on `/api/me` `is_admin` (same pattern as child pages). Non-admin → forbidden message (reuse existing admin forbidden copy pattern); do not list tools.
- **Children:** keep existing per-page `is_admin` checks (defense in depth; no shared auth context required for v1).
- No backend changes.

### Navigation

| Surface | Back target | Mechanism |
|---------|-------------|-----------|
| Hub `/admin` | `/` (customer Home) | Telegram `BackButton` + any in-page back control |
| Finance / Plans / Promos | `/admin` | Telegram `BackButton` + any in-page back control |
| Home Admin card | `/admin` | `Link` |

Do not use browser history `navigate(-1)` for these backs — explicit paths keep Telegram Mini App behavior predictable.

### Hub content (sectioned cards)

One section is enough for three tools; structure supports more later.

**Section: Shop**

| Card | Path | Existing copy keys (reuse) |
|------|------|----------------------------|
| Finance | `/admin/finance` | `admin_finance_card_title` / `admin_finance_card_subtitle` |
| Plans | `/admin/plans` | `admin_plans_card_title` / `admin_plans_card_subtitle` |
| Promos | `/admin/promos` | `admin_promos_card_title` / `admin_promos_card_subtitle` |

Card visual language: same `digital-card` pattern as Home (icon, title, subtitle, chevron, press scale, click sound).

**New i18n keys (EN + MY):**

| Key | EN intent |
|-----|-----------|
| `admin_hub_title` | Admin |
| `admin_hub_subtitle` | Manage shop tools |
| `admin_hub_section_shop` | Shop |
| `admin_card_title` | Admin |
| `admin_card_subtitle` | Finance, plans, and promos |
| `admin_hub_forbidden` | Admin access required (or reuse closest existing forbidden string if identical) |
| `admin_hub_loading` | Loading… (or reuse generic loading if already suitable) |

MY strings: grounded Burmese matching existing admin card tone in `translations.ts`.

### Layout chrome (minimal)

`AdminLayout` provides:

- Full-height container consistent with other Mini App pages
- `<Outlet />` for hub or child

It does **not** add a persistent side nav or bottom tab bar (YAGNI for three tools). Page titles stay on each child page as today.

Optional: hub-only header in `AdminHub`; children keep their own headers.

### Data flow

No new APIs. Hub only needs `/api/me` for gate. Children keep their existing fetches (`/api/revenue*`, `/api/admin/plans`, `/api/admin/promos`, etc.).

### Error handling

| Case | Behavior |
|------|----------|
| No Telegram initData | Same as other admin pages (loading ends; no data) |
| 401 on hub gate | Session expired screen / clear session (match child pattern) |
| Non-admin on hub | Forbidden message; no tool cards |
| Child errors | Unchanged per page |

## Testing

| Area | Expectation |
|------|-------------|
| `Home.test.tsx` | Admin sees **one** Admin link to `/admin`; does not see three separate Finance/Plans/Promos cards on Home; non-admin sees none |
| `App.routes.test.tsx` | Update contract to nested `/admin` registration (layout + finance/plans/promos paths still present) |
| New `AdminHub` test (or layout test) | Admin sees section + three tool links; non-admin forbidden; optional back target if testable |
| `AdminFinance` / `AdminPlans` / `AdminPromos` tests | Back navigation targets `/admin` where asserted; route mounts may use nested parent if needed for realism |
| Manual | Admin Home → Admin → each tool → BackButton returns to hub → hub BackButton to Home |

Verification commands (frontend):

```bash
cd web-app && npm test
cd web-app && npm run build
```

## File touch list (expected)

| Path | Change |
|------|--------|
| `web-app/src/App.tsx` | Nested `/admin` routes |
| `web-app/src/pages/AdminLayout.tsx` | New shell |
| `web-app/src/pages/AdminHub.tsx` | New hub |
| `web-app/src/pages/Home.tsx` | Single Admin card |
| `web-app/src/pages/AdminFinance.tsx` | Back → `/admin` |
| `web-app/src/pages/AdminPlans.tsx` | Back → `/admin` |
| `web-app/src/pages/AdminPromos.tsx` | Back → `/admin` |
| `web-app/src/lib/translations.ts` | Hub + Home Admin card keys EN/MY |
| `web-app/src/pages/Home.test.tsx` | Single Admin entry |
| `web-app/src/App.routes.test.tsx` | Nested route contract |
| `web-app/src/pages/AdminHub.test.tsx` | New (recommended) |
| Child admin `*.test.tsx` | Back path / route tree if needed |

Docs optional follow-up (not required for ship): `docs/MINI_APP.md` mention of `/admin` hub.

## Implementation approach notes

1. Add layout + hub + routes first (URLs work).
2. Point child backs to `/admin`.
3. Collapse Home to one Admin card.
4. i18n.
5. Tests + `npm test` / `npm run build`.

Prefer matching existing Home digital-card markup over inventing a new design system. No shared admin auth context in v1.

## Out of scope reminders

- Crypto Pay, live restore, payment/wallet money paths: untouched
- Telegram dual promo UX remains; Mini hub only organizes Mini tools
