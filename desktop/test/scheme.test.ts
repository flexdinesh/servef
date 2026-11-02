import assert from "node:assert/strict"
import { test } from "node:test"
import { routeSchemeURL, schemeArgument } from "../src/main/scheme.ts"

test("routeSchemeURL maps local links and keeps query and fragment", () => {
  const table: [string, ReturnType<typeof routeSchemeURL>][] = [
    ["servef://local", { connection: "local", target: "servef-app://local/" }],
    ["servef://local/", { connection: "local", target: "servef-app://local/" }],
    ["servef://local/?project=%2Fsrc%2Fa", { connection: "local", target: "servef-app://local/?project=%2Fsrc%2Fa" }],
    ["servef://local/view?project=%2Fa&path=README.md#f=1", { connection: "local", target: "servef-app://local/view?project=%2Fa&path=README.md#f=1" }],
    ["servef://local/view?wt=a%20b&x=1+2", { connection: "local", target: "servef-app://local/view?wt=a%20b&x=1+2" }],
    ["servef://ssh/host/path", { reject: "remote" }],
    ["servef://ssh", { reject: "remote" }],
    ["servef://local/api/projects", { reject: "invalid" }],
    ["servef://local/view/", { reject: "invalid" }],
    ["servef://local/_daemon/status", { reject: "invalid" }],
    ["servef://other/", { reject: "invalid" }],
    ["servef://user@local/", { reject: "invalid" }],
    ["servef://local:8080/", { reject: "invalid" }],
    ["servef-app://local/", { reject: "invalid" }],
    ["https://local/", { reject: "invalid" }],
    ["not a url", { reject: "invalid" }],
    ["", { reject: "invalid" }],
  ]
  for (const [raw, expected] of table) assert.deepEqual(routeSchemeURL(raw), expected, raw)
})

test("schemeArgument picks the first servef:// argument", () => {
  assert.equal(schemeArgument(["/app/servef", "--flag", "servef://local/view?a=1", "servef://local/"]), "servef://local/view?a=1")
  assert.equal(schemeArgument(["/app/servef", "https://x"]), null)
  assert.equal(schemeArgument([]), null)
})
