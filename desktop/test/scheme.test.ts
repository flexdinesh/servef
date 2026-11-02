import assert from "node:assert/strict"
import { test } from "node:test"
import { routeSchemeURL, schemeArgument } from "../src/main/scheme.ts"

test("routeSchemeURL maps local links and keeps query and fragment", () => {
  const table: [string, ReturnType<typeof routeSchemeURL>][] = [
    ["servef://local", { kind: "local", target: "servef-app://local/" }],
    ["servef://local/", { kind: "local", target: "servef-app://local/" }],
    ["servef://local/?project=%2Fsrc%2Fa", { kind: "local", target: "servef-app://local/?project=%2Fsrc%2Fa" }],
    ["servef://local/view?project=%2Fa&path=README.md#f=1", { kind: "local", target: "servef-app://local/view?project=%2Fa&path=README.md#f=1" }],
    ["servef://local/view?wt=a%20b&x=1+2", { kind: "local", target: "servef-app://local/view?wt=a%20b&x=1+2" }],
    ["servef://local/api/projects", { kind: "invalid" }],
    ["servef://local/view/", { kind: "invalid" }],
    ["servef://local/_daemon/status", { kind: "invalid" }],
    ["servef://other/", { kind: "invalid" }],
    ["servef://user@local/", { kind: "invalid" }],
    ["servef://user@ssh/devbox/", { kind: "invalid" }],
    ["servef://local:8080/", { kind: "invalid" }],
    ["servef-app://local/", { kind: "invalid" }],
    ["https://local/", { kind: "invalid" }],
    ["not a url", { kind: "invalid" }],
    ["", { kind: "invalid" }],
  ]
  for (const [raw, expected] of table) assert.deepEqual(routeSchemeURL(raw), expected, raw)
})

test("routeSchemeURL parses ssh links and rejects malformed ones", () => {
  assert.deepEqual(routeSchemeURL("servef://ssh/devbox/?project=%2Fsrc%2Fa&ssh-hostname=devbox&ssh-user=dee#f=1"), {
    kind: "ssh",
    link: { host: "devbox", route: "/", query: "project=%2Fsrc%2Fa", hash: "#f=1", hints: { hostname: "devbox", user: "dee" } },
  })
  assert.deepEqual(routeSchemeURL("servef://ssh/work-box/view?project=%2Fa&path=a.md&ssh-port=2222"), {
    kind: "ssh",
    link: { host: "work-box", route: "/view", query: "project=%2Fa&path=a.md", hash: "", hints: { port: 2222 } },
  })
  for (const raw of ["servef://ssh", "servef://ssh/", "servef://ssh//", "servef://ssh/devbox/api/projects", "servef://ssh/devbox/_daemon/status"]) {
    assert.deepEqual(routeSchemeURL(raw), { kind: "invalid" }, raw)
  }
})

test("schemeArgument picks the first servef:// argument", () => {
  assert.equal(schemeArgument(["/app/servef", "--flag", "servef://local/view?a=1", "servef://local/"]), "servef://local/view?a=1")
  assert.equal(schemeArgument(["/app/servef", "https://x"]), null)
  assert.equal(schemeArgument([]), null)
})
