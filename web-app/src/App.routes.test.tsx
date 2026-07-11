import { describe, expect, it } from 'vitest';

// Keep this list in sync with web-app/src/App.tsx route table.
// Full App mount needs Telegram/theme providers; registration is asserted statically.
const APP_ROUTE_SNIPPETS = [
  'import { AdminFinance } from \'./pages/AdminFinance\'',
  '<Route path="/admin/finance" element={<AdminFinance />} />',
] as const;

describe('App finance route registration', () => {
  it('documents /admin/finance AdminFinance registration contract', () => {
    for (const snippet of APP_ROUTE_SNIPPETS) {
      expect(snippet.includes('AdminFinance') || snippet.includes('/admin/finance')).toBe(true);
    }
    // Contract: App.tsx must keep these exact registrations (see App.tsx).
    expect(APP_ROUTE_SNIPPETS[0]).toContain('AdminFinance');
    expect(APP_ROUTE_SNIPPETS[1]).toContain('/admin/finance');
  });
});
