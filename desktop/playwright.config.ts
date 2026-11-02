import { defineConfig } from "@playwright/test"

// Electron smoke (Linux; Xvfb in Dagger, a real display on hosts). One app and
// one isolated daemon per spec, so one worker. SERVEF_DESKTOP_BINARY is required.
export default defineConfig({
  testDir: "./test/smoke",
  testMatch: "*.spec.ts",
  outputDir: process.env.SERVEF_DESKTOP_OUTPUT_DIR ?? "test-results",
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 120_000,
  expect: { timeout: 20_000 },
  // Per-test lines only for people watching; logs and agents get dots.
  reporter: process.stdout.isTTY ? "list" : "dot",
})
