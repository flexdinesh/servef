import assert from "node:assert/strict"
import test from "node:test"

import { shareableOrigin } from "./shareable-origin.ts"

test("the local desktop origin shares as a servef link", () => {
  assert.equal(shareableOrigin("servef-app://local"), "servef://local")
})

test("the local mygo origin shares as a servef-mygo link", () => {
  assert.equal(shareableOrigin("servef-mygo-app://local"), "servef-mygo://local")
})

test("CLI server origins share as their localhost address", () => {
  assert.equal(shareableOrigin("servef-app://cli-7971"), "http://localhost:7971")
  assert.equal(shareableOrigin("servef-app://cli-65535"), "http://localhost:65535")
})

test("http server origins on IPv4 share as their address", () => {
  assert.equal(shareableOrigin("servef-app://http-10-0-0-5-7971"), "http://10.0.0.5:7971")
  assert.equal(shareableOrigin("servef-app://http-192-168-1-20-80"), "http://192.168.1.20:80")
  assert.equal(shareableOrigin("servef-app://http-255-0-0-1-65535"), "http://255.0.0.1:65535")
})

test("ssh, other http, and unknown app origins have no shareable address", () => {
  for (const origin of [
    "servef-app://ssh-devbox", "servef-app://ssh-work-box-1a2b", "servef-app://ssh-devbox-7971",
    "servef-app://http-box-lan-80", "servef-app://http-2001-db8-5-7971", "servef-app://http-10-0-0-5-ab12-7971",
    "servef-app://http-10-0-0-256-7971", "servef-app://http-10-0-0-05-7971", "servef-app://http-10-0-0-5-65536", "servef-app://http-10-0-0-5-0",
    "servef-app://app", "servef-app://other", "servef-app://cli-0", "servef-app://cli-07971", "servef-app://cli-65536", "servef-app://cli-",
  ]) {
    assert.equal(shareableOrigin(origin), null, origin)
  }
})

test("origins outside the app are unchanged", () => {
  for (const origin of ["http://localhost:7971", "https://127.0.0.1:7972", "servef://local", "null", "", "http://servef-app://local"]) {
    assert.equal(shareableOrigin(origin), origin)
  }
})
