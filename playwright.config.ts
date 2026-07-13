import fs from 'node:fs';
import path from 'node:path';
import { defineConfig } from '@playwright/test';

const localAppData = process.env.LOCALAPPDATA ?? '';
const chromiumExecutableCandidates = [
  path.join(localAppData, 'ms-playwright', 'chromium-1217', 'chrome-win64', 'chrome.exe'),
  path.join(localAppData, 'ms-playwright', 'chromium_headless_shell-1217', 'chrome-headless-shell-win64', 'chrome-headless-shell.exe'),
];
const fallbackChromiumExecutable = chromiumExecutableCandidates.find((candidate) => fs.existsSync(candidate));

export default defineConfig({
  testDir: './e2e',
  timeout: 180_000,
  expect: {
    timeout: 15_000,
  },
  fullyParallel: false,
  workers: 1,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: process.env.L2BG_E2E_BASE_URL ?? 'http://localhost:5173',
    headless: true,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    launchOptions: fallbackChromiumExecutable ? { executablePath: fallbackChromiumExecutable } : undefined,
  },
});
