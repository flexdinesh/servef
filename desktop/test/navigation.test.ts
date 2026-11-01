import assert from "node:assert/strict"
import { test } from "node:test"
import { navigationPolicy, permissionAllowed } from "../src/main/navigation.ts"

const origin = "servef-app://local"

test("navigationPolicy allows the connection origin, routes servef:, opens web links externally", () => {
  const table: [string, ReturnType<typeof navigationPolicy>][] = [
    ["servef-app://local/view?project=%2Fa", { kind: "allow" }],
    ["servef-app://local/", { kind: "allow" }],
    ["servef://local/view?project=%2Fa", { kind: "scheme", url: "servef://local/view?project=%2Fa" }],
    ["https://github.com/x", { kind: "external", url: "https://github.com/x" }],
    ["http://example.com/", { kind: "external", url: "http://example.com/" }],
    ["mailto:a@b.c", { kind: "external", url: "mailto:a@b.c" }],
    ["servef-app://ssh-box/", { kind: "deny" }],
    ["servef-app://user@local/", { kind: "deny" }],
    ["file:///etc/passwd", { kind: "deny" }],
    ["javascript:alert(1)", { kind: "deny" }],
    ["data:text/html,<p>", { kind: "deny" }],
    ["about:blank", { kind: "deny" }],
    ["chrome://settings", { kind: "deny" }],
    ["::nope", { kind: "deny" }],
  ]
  for (const [url, expected] of table) assert.deepEqual(navigationPolicy(url, origin), expected, url)
})

test("permissionAllowed allows sanitized clipboard writes only", () => {
  assert.equal(permissionAllowed("clipboard-sanitized-write"), true)
  for (const permission of ["clipboard-read", "notifications", "media", "geolocation", "openExternal", ""]) assert.equal(permissionAllowed(permission), false, permission)
})
