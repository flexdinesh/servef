import { defineConfig, devices } from "@playwright/test"

const apiPort = 18080
const tldrawApiPort = 18081
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
      command: "go run . testdata/markdown --port 18082 --no-open --file-type md",
      reuseExistingServer: false,
      timeout: 120_000,
      url: "http://127.0.0.1:18082/api/page",
    },
    {
      command: `go run . testdata/markdown --port ${apiPort} --no-open`,
      reuseExistingServer: false,
      timeout: 120_000,
      url: `http://127.0.0.1:${apiPort}/api/page`,
    },
    {
      command: `go run . testdata/markdown --port ${tldrawApiPort} --no-open --feature mermaid-tldraw`,
      reuseExistingServer: false,
      timeout: 120_000,
      url: `http://127.0.0.1:${tldrawApiPort}/api/features`,
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
