import { defineConfig, devices } from "@playwright/test";

/**
 * Meta App Review screencasts (CF-2). Runs the production build of the panel (`pnpm review:build`)
 * against the fake Go API in mock-api.mjs and records one video per composition. Ports sit outside
 * the usual dev range so a local stack can keep running.
 */
const PANEL_PORT = Number(process.env.REVIEW_PANEL_PORT ?? 3100);
const MOCK_PORT = Number(process.env.REVIEW_MOCK_PORT ?? 18090);
const PANEL_URL = `http://127.0.0.1:${PANEL_PORT}`;
export const MOCK_URL = `http://127.0.0.1:${MOCK_PORT}`;
// Full HD by default; REVIEW_SIZE=1280x800 for a lighter run (e.g. CI runners without a GPU).
const [width, height] = (process.env.REVIEW_SIZE ?? "1920x1080")
  .split("x")
  .map(Number);
const size = { width, height };

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.review.ts",
  outputDir: "./out/raw",
  timeout: 240_000,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [["list"]],
  use: {
    ...devices["Desktop Chrome"],
    baseURL: PANEL_URL,
    viewport: size,
    video: { mode: "on", size },
    screenshot: "off",
    trace: "retain-on-failure",
    locale: "en-US",
    timezoneId: "Europe/London",
    serviceWorkers: "block",
    launchOptions: { slowMo: 60 },
  },
  webServer: [
    {
      command: "node e2e/meta-review/mock-api.mjs",
      cwd: "../..",
      url: `${MOCK_URL}/healthz`,
      reuseExistingServer: false,
      timeout: 15_000,
      env: {
        REVIEW_MOCK_PORT: String(MOCK_PORT),
        REVIEW_PANEL_ORIGIN: PANEL_URL,
      },
    },
    {
      command: `pnpm exec next start -H 127.0.0.1 -p ${PANEL_PORT}`,
      cwd: "../..",
      url: `${PANEL_URL}/login`,
      reuseExistingServer: false,
      timeout: 120_000,
      env: {
        API_URL: `${MOCK_URL}/v1`,
        AUTH_URL: PANEL_URL,
        AUTH_TRUST_HOST: "true",
        // Throwaway values for the local NextAuth cookie; nothing here reaches a real service.
        AUTH_SECRET: "meta-review-only-auth-secret-0000000000",
        AUTH_ADAPTER_SECRET: "meta-review-only-adapter-secret",
        NEXT_TELEMETRY_DISABLED: "1",
      },
    },
  ],
});
