import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Assert nested admin registration against real App.tsx source.
const appSource = readFileSync(resolve(__dirname, './App.tsx'), 'utf8');

// The admin and reseller surface is split out of the first-paint bundle, so
// each of these modules is imported lazily and rendered inside one Suspense.
const LAZY_ROUTE_MODULES = [
  { exportName: 'AdminLayout', modulePath: './pages/AdminLayout' },
  { exportName: 'AdminHub', modulePath: './pages/AdminHub' },
  { exportName: 'AdminFinance', modulePath: './pages/AdminFinance' },
  { exportName: 'AdminPlans', modulePath: './pages/AdminPlans' },
  { exportName: 'AdminPromos', modulePath: './pages/AdminPromos' },
  { exportName: 'AdminResellers', modulePath: './pages/AdminResellers' },
  { exportName: 'ResellerAccount', modulePath: './pages/ResellerAccount' },
] as const;

const REQUIRED = [
  "import { lazy, Suspense } from 'react'",
  '<Route path="/admin" element={<AdminLayout />}>',
  '<Route index element={<AdminHub />} />',
  '<Route path="finance" element={<AdminFinance />} />',
  '<Route path="plans" element={<AdminPlans />} />',
  '<Route path="promos" element={<AdminPromos />} />',
  '<Suspense fallback={<RouteLoadingSkeleton />}>',
] as const;

describe('App admin route registration', () => {
  it('registers nested /admin layout + hub + children in App.tsx', () => {
    for (const snippet of REQUIRED) {
      expect(appSource).toContain(snippet);
    }
  });

  it('loads the admin and reseller pages lazily instead of in the first bundle', () => {
    for (const { exportName, modulePath } of LAZY_ROUTE_MODULES) {
      expect(appSource).toContain(
        `const ${exportName} = lazy(() => import('${modulePath}').then((m) => ({ default: m.${exportName} })));`,
      );
      expect(appSource).not.toContain(`import { ${exportName} } from '${modulePath}'`);
    }
  });

  it('keeps the purchase path eager so the money flow has no extra chunk round trip', () => {
    for (const eager of ['Home', 'Plans', 'Checkout', 'Wallet'] as const) {
      expect(appSource).toContain(`import { ${eager} } from './pages/${eager}'`);
      expect(appSource).not.toContain(`lazy(() => import('./pages/${eager}')`);
    }
  });
});
