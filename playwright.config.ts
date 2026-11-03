import { defineConfig, devices } from "@playwright/test"

import { webPort } from "./tests/browser/ports.ts"

export default defineConfig({
  testDir: "./tests/browser",
  fullyParallel: true,
  outputDir: process.env.SERVEF_BROWSER_OUTPUT_DIR ?? "test-results",
  // Bound CPU and Chromium memory across agent sessions. --workers overrides.
  workers: 2,
  retries: 0,
  // Per-test lines only for people watching; logs and agents get dots.
  reporter: process.stdout.isTTY ? "list" : "dot",
  use: {
    baseURL: `http://127.0.0.1:${webPort}`,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  // chromium is the gate (check:pr, test:browser). webkit is opt-in
  // (test:browser:webkit) and reports compatibility only.
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "webkit", use: { ...devices["Desktop Safari"] } },
  ],
})
