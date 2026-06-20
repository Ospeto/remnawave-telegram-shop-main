# Plan 007: Refresh frontend dependencies and clear audit/tooling drift

> **Executor instructions**: Follow this plan step by step. Run every verification command and confirm the expected result before moving to the next step. If anything in the "STOP conditions" section occurs, stop and report - do not improvise. When done, update the status row for this plan in `plans/README.md` unless a reviewer dispatched you and told you they maintain the index.
>
> **Drift check (run first)**: `git diff --stat 8e80b0b..HEAD -- web-app/package.json web-app/package-lock.json web-app/vite.config.ts web-app/src`
> If any in-scope file changed since this plan was written, compare the "Current state" excerpts against the live code before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S-M
- **Risk**: LOW-MED
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `8e80b0b`, 2026-06-20

## Why This Matters

The frontend tests pass, but dependency signals are drifting. `npm audit --omit=dev --audit-level=high` reports moderate React Router advisories through `react-router-dom@6.30.3`, and Vitest prints Vite/plugin deprecation warnings during test startup. Clearing this while the app is small keeps future frontend changes cheaper.

## Current State

- `web-app/package.json` - declares React, React Router, Vite, Vitest, and plugin versions.
- `web-app/package-lock.json` - currently resolves `react-router-dom` and `react-router` to `6.30.3`.
- `web-app/vite.config.ts` - minimal Vite config with React plugin and dev proxy.

Current excerpts:

```json
// web-app/package.json:12
"dependencies": {
    "react": "^18.2.0",
    "react-dom": "^18.2.0",
    "react-router-dom": "^6.22.0"
},
"devDependencies": {
    "@vitejs/plugin-react": "^4.2.1",
    "typescript": "^5.2.2",
    "vite": "^5.1.4",
    "vitest": "^4.1.2"
}
```

```json
// web-app/package-lock.json:3049
"node_modules/react-router-dom": {
    "version": "6.30.3",
    ...
    "dependencies": {
        "@remix-run/router": "1.23.2",
        "react-router": "6.30.3"
    }
}
```

```json
// web-app/package-lock.json:3412
"node_modules/vite": {
    "version": "5.4.21",
```

```ts
// web-app/vite.config.ts:5
export default defineConfig({
    plugins: [react()],
    test: {
        environment: 'jsdom',
        setupFiles: './src/test/setup.ts',
    },
    server: {
        port: 5173,
        proxy: {
            '/api': {
                target: 'http://localhost:8080',
```

Observed verification output on 2026-06-20:

- `npm test` passes but prints Vite warnings about deprecated `esbuild` and `optimizeDeps.esbuildOptions` options.
- `npm audit --omit=dev --audit-level=high` exits 0 but reports 2 moderate advisories for React Router same-origin redirect handling.

Repo conventions:

- Frontend package manager is npm with `package-lock.json`.
- CI uses Node 20, `npm ci`, `npm test`, and `npm run build`.

## Commands You Will Need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Install exact deps | `cd web-app && npm ci` | exit 0 |
| Audit baseline | `cd web-app && npm audit --omit=dev --audit-level=moderate` | currently fails before fix, should exit 0 after fix |
| Frontend tests | `cd web-app && npm test` | exit 0 and no Vite deprecation warning if tooling drift is fixed |
| Frontend build | `cd web-app && npm run build` | exit 0 |

## Scope

**In scope**:
- `web-app/package.json`
- `web-app/package-lock.json`
- `web-app/vite.config.ts` only if current dependency versions require config cleanup
- Frontend source files only if a dependency upgrade requires a small router API compatibility fix

**Out of scope**:
- Do not migrate React 18 to a new major version unless React Router requires it.
- Do not rewrite routing structure.
- Do not change backend APIs.
- Do not switch package managers.

## Git Workflow

- Branch: `codex/007-frontend-dependency-drift`
- Commit message: `chore: refresh frontend dependencies`
- Do not push or open a PR unless instructed.

## Steps

### Step 1: Refresh React Router with the smallest safe change

From `web-app/`, run:

```bash
npm audit fix
```

If `npm audit fix` proposes a major upgrade that rewrites routing APIs, stop and report. Otherwise accept the minimal fix that updates `react-router` and `react-router-dom` beyond the vulnerable range.

If `npm audit fix` cannot resolve it, manually install the latest compatible non-vulnerable `react-router-dom` version:

```bash
npm install react-router-dom@latest
```

Then inspect the diff. If this jumps to a new major version and the app no longer compiles, stop unless the required changes are limited to straightforward imports or route wrapper updates.

**Verify**: `cd web-app && npm audit --omit=dev --audit-level=moderate` -> exit 0.

### Step 2: Align Vite, plugin, and Vitest versions

Run:

```bash
cd web-app
npm outdated vite @vitejs/plugin-react vitest typescript
```

Choose the smallest coherent set that removes the test-time deprecation warnings. Prefer keeping major versions aligned with what Vitest supports. At the time of planning, lockfile evidence shows Vitest 4.1.2 expects Vite `^6.0.0 || ^7.0.0 || ^8.0.0`, while the app resolves Vite 5.4.21.

Update `package.json` and `package-lock.json` through npm commands, not manual lockfile edits. Example command shape:

```bash
npm install -D vite@latest @vitejs/plugin-react@latest vitest@latest typescript@latest
```

If the latest versions require Node newer than CI's Node 20, stop and report instead of breaking CI.

**Verify**: `cd web-app && npm test` -> exit 0. The prior Vite deprecation warnings should be gone or explicitly explained in the plan status update if an upstream package still emits them.

### Step 3: Keep `vite.config.ts` minimal

Inspect `web-app/vite.config.ts`. Do not add obsolete `esbuild` or `optimizeDeps.esbuildOptions` settings. If the upgraded toolchain needs config changes, keep them limited to documented Vite/Vitest options and preserve:

- `plugins: [react()]`
- `test.environment = 'jsdom'`
- `test.setupFiles = './src/test/setup.ts'`
- the `/api` dev proxy to `http://localhost:8080`

**Verify**: `cd web-app && npm run build` -> exit 0.

### Step 4: Run frontend regression checks

Run:

```bash
cd web-app
npm test
npm run build
npm audit --omit=dev --audit-level=moderate
```

Expected result: all exit 0. If the audit still has only low-severity findings, record that in the plan status update and do not expand scope.

## Test Plan

- Existing Vitest suite covers current frontend behavior.
- Existing production build verifies TypeScript and Vite compatibility.
- Audit command verifies the React Router advisory is cleared.

## Done Criteria

- [ ] `web-app/package.json` and `web-app/package-lock.json` are updated through npm.
- [ ] `cd web-app && npm audit --omit=dev --audit-level=moderate` exits 0, or any remaining advisory is documented as non-runtime and accepted by a reviewer.
- [ ] `cd web-app && npm test` exits 0.
- [ ] `cd web-app && npm run build` exits 0.
- [ ] Vite deprecation warnings from the original audit are gone, or an upstream-blocked warning is documented with exact package/version evidence.
- [ ] No backend files are modified.
- [ ] `plans/README.md` status row updated.

## STOP Conditions

Stop and report back if:

- The required React Router fix demands a broad route migration.
- Latest tooling requires a Node version newer than CI's Node 20.
- `npm audit fix` changes unrelated packages in a way that breaks tests or build.
- More than a small compatibility edit is needed in frontend source.

## Maintenance Notes

Keep frontend dependency refreshes small and frequent. When CI's Node version changes, revisit Vite/Vitest compatibility together rather than upgrading one tool at a time.
