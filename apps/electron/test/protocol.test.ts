import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import { test } from "node:test"
import { EXPECTED_PROTOCOL } from "../src/main/daemon-contract.ts"

test("EXPECTED_PROTOCOL matches the daemon's ProtocolVersion", () => {
  const source = readFileSync(new URL("../../servef/internal/daemon/daemon.go", import.meta.url), "utf8")
  const match = /^const ProtocolVersion = (\d+)$/m.exec(source)
  assert.ok(match !== null, "daemon.go declares const ProtocolVersion = N")
  assert.equal(Number(match[1]), EXPECTED_PROTOCOL)
})
