import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: 'html',
  use: {
    baseURL: 'http://localhost:8080',
    trace: 'on-first-retry',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: 'make run',
    cwd: '..',
    url: 'http://localhost:8080/health',
    reuseExistingServer: !process.env.CI,
    timeout: 30000,
    env: {
      // Increase rate limit for E2E tests to avoid 429 errors
      RATE_LIMIT_RPM: '600',
    },
  },
});
