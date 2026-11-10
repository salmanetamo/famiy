import { defineConfig, devices } from '@playwright/test';

/**
 * End-to-end smoke test. It drives the real app (Vite) against the real API and database, so both
 * must be running: `go run` (or the dev container) for the API on :8080 and Postgres behind it.
 * It is not part of `npm run check` and not yet part of CI, which has no database for it.
 *
 * Vite is started here if it is not already running; the API is not (see e2e/support.ts for the
 * check that explains a missing API).
 */
export default defineConfig({
  testDir: 'e2e',
  // one account, so one test at a time
  fullyParallel: false,
  workers: 1,
  retries: 0,
  timeout: 90_000,
  reporter: [['list']],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:5173',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'npm run dev',
    url: process.env.E2E_BASE_URL ?? 'http://localhost:5173',
    reuseExistingServer: true,
    timeout: 30_000,
  },
});
