import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: '.',
  testMatch: '*.spec.js',
  timeout: 30000,
  workers: 1,
  use: {
    baseURL: 'https://localhost:8080',
    ignoreHTTPSErrors: true,
    headless: true,
  },
});
