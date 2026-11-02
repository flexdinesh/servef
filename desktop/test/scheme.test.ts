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

test("routeSchemeURL maps server links: loopback to cli, other hosts to http", () => {
  const table: [string, ReturnType<typeof routeSchemeURL>][] = [
    ["servef://localhost:7971/", { kind: "cli", port: 7971, target: "servef-app://cli-7971/" }],
    ["servef://LOCALHOST:7971/view?project=%2Fa#f=1", { kind: "cli", port: 7971, target: "servef-app://cli-7971/view?project=%2Fa#f=1" }],
    ["servef://127.0.0.1:80", { kind: "cli", port: 80, target: "servef-app://cli-80/" }],
    ["servef://127.1:7972/", { kind: "cli", port: 7972, target: "servef-app://cli-7972/" }],
    ["servef://[::1]:7973/", { kind: "cli", port: 7973, target: "servef-app://cli-7973/" }],
    ["servef://10.0.0.5:7971/view?project=%2Fsrv&path=a.md#L3", { kind: "http", host: "10.0.0.5", port: 7971, path: "/view?project=%2Fsrv&path=a.md#L3" }],
    ["servef://Box.Lan:8080/?x=1+2", { kind: "http", host: "box.lan", port: 8080, path: "/?x=1+2" }],
    ["servef://[2001:DB8::5]:7971", { kind: "http", host: "[2001:db8::5]", port: 7971, path: "/" }],
  ]
  for (const [raw, expected] of table) assert.deepEqual(routeSchemeURL(raw), expected, raw)
})

test("routeSchemeURL rejects server links without a port, with credentials, or off / and /view", () => {
  for (const raw of [
    "servef://10.0.0.5/", "servef://10.0.0.5:0/", "servef://10.0.0.5:65536/", "servef://u@10.0.0.5:7971/", "servef://u:p@10.0.0.5:7971/",
    "servef://10.0.0.5:7971/api/projects", "servef://10.0.0.5:7971/_daemon/status", "servef://10.0.0.5:7971/_servef/control",
    "servef://a%20b:7971/", "servef://ssh/devbox/", "servef://ssh/devbox/?ssh-user=dee", "servef://ssh",
  ]) assert.deepEqual(routeSchemeURL(raw), { kind: "invalid" }, raw)
})

test("schemeArgument picks the first servef:// argument", () => {
  assert.equal(schemeArgument(["/app/servef", "--flag", "servef://local/view?a=1", "servef://local/"]), "servef://local/view?a=1")
  assert.equal(schemeArgument(["/app/servef", "https://x"]), null)
  assert.equal(schemeArgument([]), null)
})
