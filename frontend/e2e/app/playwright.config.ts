import { defineConfig, devices } from "@playwright/test";

/**
 * Launch-critical E2E suite (CF-131). Runs against a running stack (Go API + `next start`) whose
 * database holds the demo organization from `seed-demo` (docs/demo-org.md). The stack is started
 * outside Playwright: locally by hand, in CI by .github/workflows/e2e.yml.
 *
 * Env: E2E_BASE_URL (default http://127.0.0.1:3000), E2E_DEMO_PASSWORD (the DEMO_PASSWORD given to
 * seed-demo), E2E_ADMIN_EMAIL / E2E_ADMIN_PASSWORD (a platform super admin).
 */
export const BASE_URL = process.env.E2E_BASE_URL ?? "http://127.0.0.1:3000";

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.e2e.ts",
  outputDir: "./out",
  timeout: 120_000,
  expect: { timeout: 15_000 },
  // One browser, one worker: the flows share the demo organization and the machine stays quiet.
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  forbidOnly: !!process.env.CI,
  reporter: process.env.CI
    ? [["list"], ["html", { outputFolder: "./report", open: "never" }]]
    : [["list"]],
  use: {
    ...devices["Desktop Chrome"],
    // Wide enough for the three-pane inbox (list, conversation, contact sidebar).
    viewport: { width: 1920, height: 1080 },
    baseURL: BASE_URL,
    actionTimeout: 15_000,
    locale: "tr-TR",
    timezoneId: "Europe/Istanbul",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
    serviceWorkers: "block",
  },
  projects: [
    { name: "setup", testMatch: "**/auth.setup.ts" },
    { name: "app", dependencies: ["setup"] },
  ],
});
