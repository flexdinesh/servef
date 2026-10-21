import assert from "node:assert/strict"
import test from "node:test"

import {
  devServerUrls,
  formatDevServerUrl,
  mockFixtureName,
  primaryLanIpv4,
  shouldOpenBrowser,
} from "./dev-server.ts"

test("primaryLanIpv4 returns only the first external IPv4 address", () => {
  assert.equal(
    primaryLanIpv4({
      lo: [{ address: "127.0.0.1", family: "IPv4", internal: true }],
      wifi: [
        { address: "fe80::1", family: "IPv6", internal: false },
        { address: "192.168.1.25", family: "IPv4", internal: false },
      ],
      vpn: [{ address: "10.0.0.2", family: "IPv4", internal: false }],
    }),
    "192.168.1.25",
  )
})

test("primaryLanIpv4 returns undefined without an external IPv4 address", () => {
  assert.equal(
    primaryLanIpv4({
      lo: [{ address: "127.0.0.1", family: "IPv4", internal: true }],
      wifi: [{ address: "fe80::1", family: "IPv6", internal: false }],
    }),
    undefined,
  )
})

test("devServerUrls uses the selected port and at most one LAN address", () => {
  assert.deepEqual(devServerUrls(5174, "192.168.1.25"), [
    { label: "Local", url: "http://localhost:5174/" },
    { label: "Host", url: "http://0.0.0.0:5174/" },
    { label: "Network", url: "http://192.168.1.25:5174/" },
  ])
  assert.deepEqual(devServerUrls(5174, undefined), [
    { label: "Local", url: "http://localhost:5174/" },
    { label: "Host", url: "http://0.0.0.0:5174/" },
  ])
})

test("formatDevServerUrl aligns startup labels", () => {
  assert.equal(
    formatDevServerUrl({ label: "Local", url: "http://localhost:5173/" }),
    "  ➜  Local:   http://localhost:5173/",
  )
  assert.equal(
    formatDevServerUrl({ label: "Network", url: "http://192.168.1.25:5173/" }),
    "  ➜  Network: http://192.168.1.25:5173/",
  )
})

test("shouldOpenBrowser permits only frontend dev lifecycles outside SSH", () => {
  assert.equal(shouldOpenBrowser({ npm_lifecycle_event: "dev" }), true)
  assert.equal(shouldOpenBrowser({ npm_lifecycle_event: "dev:mock" }), true)
  assert.equal(
    shouldOpenBrowser({ npm_lifecycle_event: "dev", SSH_TTY: "" }),
    true,
  )
  assert.equal(shouldOpenBrowser({ npm_lifecycle_event: "test:browser" }), false)
  assert.equal(shouldOpenBrowser({}), false)

  for (const sshVariable of ["SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"]) {
    assert.equal(
      shouldOpenBrowser({
        npm_lifecycle_event: "dev",
        [sshVariable]: "active",
      }),
      false,
    )
  }
})

test("mockFixtureName maps supported API scenarios", () => {
  assert.deepEqual(mockFixtureName("GET", "/page"), {
    name: "page-index.json",
    status: 200,
  })
  assert.deepEqual(
    mockFixtureName("GET", "/page?path=guides%2Fgetting-started.md"),
    { name: "page-document.json", status: 200 },
  )
  assert.deepEqual(mockFixtureName("GET", "/page?path=missing.md"), {
    name: "page-missing.json",
    status: 404,
  })
  assert.deepEqual(mockFixtureName("GET", "/page?path=assets%2Fsample.bin"), {
    name: "page-binary.json",
    status: 200,
  })
  assert.deepEqual(mockFixtureName("GET", "/page?path=logs%2Fenormous.log"), {
    name: "page-too-large.json",
    status: 200,
  })
  assert.deepEqual(mockFixtureName("GET", "/format?path=config%2Fsettings.json"), {
    name: "format-settings.json",
    status: 200,
  })
  assert.deepEqual(mockFixtureName("GET", "/format?path=config%2Fbroken.json"), {
    name: "format-error.json",
    status: 422,
  })
  assert.deepEqual(mockFixtureName("GET", "/page?path=navigation%2Fgreet.go"), { name: "page-go.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/definition?path=navigation%2Fgreet.go&line=6&column=8"), { name: "definition.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/references?path=navigation%2Fgreet.go&line=6&column=8"), { name: "references.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/navigation-status?path=navigation%2Fgreet.go"), { name: "navigation-status.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/raw?path=assets%2Fpreview.png"), {
    name: "raw-preview.png",
    status: 200,
    contentType: "image/png",
  })
  assert.deepEqual(mockFixtureName("GET", "/raw?path=guides%2Fgetting-started.md"), {
    name: "error-not-found.json",
    status: 404,
  })
  for (const endpoint of ["page", "format", "raw"]) {
    for (const key of ["constructor", "__proto__", "toString"]) {
      assert.equal(mockFixtureName("GET", `/${endpoint}?path=${key}`)?.status, 404, `${endpoint} ${key}`)
    }
  }
  assert.deepEqual(mockFixtureName("GET", "/changes?project=%2Fwork"), { name: "changes.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/diff?project=%2Fwork&path=code%2Fmain.go"), { name: "diff-modified.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/diff?path=config%2Fservef.toml"), { name: "diff-deleted.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/diff?path=guides%2Fnew-guide.md"), { name: "diff-untracked.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/diff?path=guides%2Fmissing.md"), { name: "error-not-found.json", status: 404 })
  assert.deepEqual(mockFixtureName("GET", "/diff?path=constructor"), { name: "error-not-found.json", status: 404 })
  assert.deepEqual(mockFixtureName("GET", "/projects"), { name: "projects.json", status: 200 })
  assert.deepEqual(mockFixtureName("GET", "/directories?path=servef"), { name: "directories.json", status: 200 })
  assert.equal(mockFixtureName("POST", "/page"), undefined)
  assert.equal(mockFixtureName("GET", "/unknown"), undefined)
})
