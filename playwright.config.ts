import { defineConfig, devices } from "@playwright/test";

const adminURL = process.env.E2E_ADMIN_URL ?? "http://localhost:5173";
const webURL = process.env.E2E_WEB_URL ?? "http://localhost:4321";

// The suite drives an already running development stack (API :8080, admin :5173,
// web :4321). Fixtures are created through the real API so permissions, media and
// review rules are exercised exactly as in production.
export default defineConfig({
  testDir: "./tests/e2e",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: Boolean(process.env.CI),
  reporter: [
    ["list"],
    ["html", { outputFolder: "playwright-report", open: "never" }],
  ],
  use: {
    baseURL: adminURL,
    locale: "zh-CN",
    timezoneId: "Asia/Shanghai",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "off",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], baseURL: adminURL },
      metadata: { webURL },
    },
  ],
});
