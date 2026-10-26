import { execFileSync } from "node:child_process"

import { defineConfig, devices } from "@playwright/test"

import { gitFixtureRoot } from "./tests/browser/git-fixture.ts"
import { apiPort, filteredApiOrigin, filteredApiPort, gitApiOrigin, gitApiPort } from "./tests/browser/ports.ts"

const webPort = 15173

// Walkthroughs the specs publish land beside the git fixture, not in the
// user's cache (Linux; elsewhere the specs delete their own). Go keeps its
// build cache.
const gitServerEnv = {
  ...process.env,
  XDG_CACHE_HOME: `${gitFixtureRoot}-cache`,
  GOCACHE: process.env.GOCACHE || execFileSync("go", ["env", "GOCACHE"], { encoding: "utf8" }).trim(),
}

export default defineConfig({
  testDir: "./tests/browser",
  // Spread tests, not just files, across workers; app.spec.ts alone otherwise
  // bounds the run.
  fullyParallel: true,
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
      // A throwaway git repository with one change of every kind.
      command: `node tests/browser/git-fixture.ts && go run . '${gitFixtureRoot}' --port ${gitApiPort} --no-open`,
      env: gitServerEnv,
      reuseExistingServer: false,
      timeout: 120_000,
      url: `${gitApiOrigin}/api/page`,
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
