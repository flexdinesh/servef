import { defineConfig, devices } from "@playwright/test"

import { apiPort, filteredApiOrigin, filteredApiPort } from "./tests/browser/ports.ts"

const webPort = 15173

export default defineConfig({
  testDir: "./tests/browser",
  retries: 0,
  use: {
    baseURL: `http://127.0.0.1:${webPort}`,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: [
    {
      command: `go run . testdata/markdown --port ${filteredApiPort} --no-open --filename '*.md'`,
      reuseExistingServer: false,
      timeout: 120_000,
      url: `${filteredApiOrigin}/api/page`,
    },
    {
      command: `go run . testdata/markdown --port ${apiPort} --no-open`,
      reuseExistingServer: false,
      timeout: 120_000,
      url: `http://127.0.0.1:${apiPort}/api/page`,
    },
    {
      command: `pnpm --filter @servef/web exec vite --configLoader native --host 127.0.0.1 --port ${webPort} --strictPort`,
      env: {
        ...process.env,
        SERVEF_API_TARGET: `http://127.0.0.1:${apiPort}`,
      },
      reuseExistingServer: false,
      timeout: 120_000,
      url: `http://127.0.0.1:${webPort}`,
    },
  ],
})
