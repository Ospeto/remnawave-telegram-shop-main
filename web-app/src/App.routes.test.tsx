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
