import assert from "node:assert/strict"
import test from "node:test"

import { requestedPort, serverPort, startProcess, stopProcess } from "../scripts/browser-process.ts"

test("parallel servers retain distinct OS-assigned ports until stopped", async () => {
  const source = `
    const http = require('node:http');
    const server = http.createServer((request, response) => response.end('ready'));
    server.listen(0, '127.0.0.1', () => console.log('http://localhost:' + server.address().port + '/'));
  `
  const first = startProcess(process.execPath, ["-e", source], process.env, true)
  const second = startProcess(process.execPath, ["-e", source], process.env, true)
  try {
    const [firstPort, secondPort] = await Promise.all([serverPort(first), serverPort(second)])
    assert.notEqual(firstPort, secondPort)
    assert.equal(await (await fetch(`http://127.0.0.1:${firstPort}`)).text(), "ready")
    await stopProcess(first)
    assert.equal(await (await fetch(`http://127.0.0.1:${secondPort}`)).text(), "ready")
  } finally {
    await Promise.all([stopProcess(first), stopProcess(second)])
  }
})

test("startup failure rejects readiness and cleanup handles missing commands", async () => {
  const failed = startProcess(process.execPath, ["-e", "process.exit(7)"], process.env, true)
  await assert.rejects(serverPort(failed), /status 7/)
  await stopProcess(failed)
  const missing = startProcess("servef-browser-deliberately-missing-command", [], process.env, true)
  await assert.rejects(serverPort(missing), /ENOENT/)
  await stopProcess(missing)
})

test("cleanup stops descendants after their launcher exits", { skip: process.platform === "win32" }, async () => {
  const server = `
    process.on('SIGTERM', () => {});
    const server = require('node:http').createServer((request, response) => response.end('ready'));
    server.listen(0, '127.0.0.1', () => console.log('http://127.0.0.1:' + server.address().port + '/'));
  `
  const launcher = `
    process.on('SIGTERM', () => process.exit(0));
    const child = require('node:child_process').spawn(process.execPath, ['-e', ${JSON.stringify(server)}], { stdio: ['ignore', 'pipe', 'ignore'] });
    child.stdout.pipe(process.stdout);
  `
  const proc = startProcess(process.execPath, ["-e", launcher], process.env, true)
  try {
    const port = await serverPort(proc)
    await stopProcess(proc)
    await assert.rejects(fetch(`http://127.0.0.1:${port}`))
  } finally {
    await stopProcess(proc)
  }
})

test("port overrides preserve explicit ports and reject invalid ranges", () => {
  assert.equal(requestedPort(undefined, "PORT"), 0)
  assert.equal(requestedPort("0", "PORT"), 0)
  assert.equal(requestedPort("18090", "PORT", 65532), 18090)
  for (const value of ["-1", "65533", "NaN", "12.5"]) {
    assert.throws(() => requestedPort(value, "PORT", 65532), /PORT must be a port/)
  }
})
