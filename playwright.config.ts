import { defineConfig, devices } from "@playwright/test"

import { webPort } from "./tests/browser/ports.ts"

export default defineConfig({
  testDir: "./tests/browser",
  fullyParallel: true,
  // Bound CPU and Chromium memory across agent sessions. --workers overrides.
  workers: 2,
  retries: 0,
  use: {
    baseURL: `http://127.0.0.1:${webPort}`,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
})
