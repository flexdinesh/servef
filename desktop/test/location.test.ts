import assert from "node:assert/strict"
import { test } from "node:test"
import { NOT_A_SERVEF_LINK, parsePastedLocation, PASTE_LIMIT } from "../src/main/location.ts"

test("loopback http URLs on / and /view map to cli origins with query and fragment", () => {
  const cases: [string, number, string][] = [
    ["http://localhost:7971/", 7971, "servef-app://cli-7971/"],
    ["  http://localhost:7971/?project=%2Fsrc%2Fapp&path=README.md#L3  ", 7971, "servef-app://cli-7971/?project=%2Fsrc%2Fapp&path=README.md#L3"],
    ["http://127.0.0.1:7972/view?project=/src&wt=abc", 7972, "servef-app://cli-7972/view?project=/src&wt=abc"],
    ["http://[::1]:7973", 7973, "servef-app://cli-7973/"],
    ["HTTP://LOCALHOST:7971/", 7971, "servef-app://cli-7971/"],
    ["http://localhost/", 80, "servef-app://cli-80/"],
  ]
  for (const [raw, port, target] of cases) assert.deepEqual(parsePastedLocation(raw), { kind: "cli", port, target }, raw)
})

test("servef links pass through for the scheme handler", () => {
  for (const raw of ["servef://local/?project=%2Fsrc", "servef://ssh/devbox/view?project=/src&wt=1"]) {
    assert.deepEqual(parsePastedLocation(` ${raw} `), { kind: "scheme", url: raw }, raw)
  }
})

test("everything else is not a servef link", () => {
  const invalid = { kind: "invalid", reason: NOT_A_SERVEF_LINK }
  for (const raw of [
    "",
    "   ",
    "localhost:7971",
    "https://localhost:7971/",
    "http://example.com:7971/",
    "http://localhost.example:7971/",
    "http://192.168.1.5:7971/",
    "http://[::2]:7971/",
    "http://user:pass@localhost:7971/",
    "servef://user@local/",
    "http://localhost:7971/assets/index.js",
    "http://localhost:7971/api/projects",
    "http://localhost:7971/_servef/control",
    "http://localhost:0/",
    "http://localhost:70000/",
    "file:///etc/passwd",
    "javascript:alert(1)",
    `http://localhost:7971/?q=${"a".repeat(PASTE_LIMIT)}`,
  ]) assert.deepEqual(parsePastedLocation(raw), invalid, raw)
})
