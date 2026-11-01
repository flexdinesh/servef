import assert from "node:assert/strict"
import { test } from "node:test"
import { isLocalFrame } from "../src/main/bridge.ts"

test("isLocalFrame accepts servef-app://local frames only", () => {
  for (const url of ["servef-app://local/", "servef-app://local/view?project=%2Fa#x"]) {
    assert.equal(isLocalFrame(url), true, url)
  }
  for (const url of [
    "servef-app://ssh-box/",
    "servef-app://app/connect",
    "servef-app://local.evil/",
    "servef-app://user@local/",
    "servef://local/",
    "https://local/",
    "http://localhost/",
    "file:///local/",
    "about:blank",
    "",
    "nope",
  ]) {
    assert.equal(isLocalFrame(url), false, url)
  }
})
