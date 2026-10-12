import assert from "node:assert/strict"
import test from "node:test"

import { fitScale, maximumZoom, minimumZoom, zoomIn, zoomOut, zoomPercent } from "./image-zoom.ts"

test("fit never enlarges and shrinks to the tighter axis", () => {
  assert.equal(fitScale({ width: 200, height: 100 }, { width: 800, height: 600 }), 1)
  assert.equal(fitScale({ width: 1920, height: 1080 }, { width: 960, height: 1080 }), 0.5)
  assert.equal(fitScale({ width: 1000, height: 2000 }, { width: 1000, height: 500 }), 0.25)
  assert.equal(fitScale({ width: 0, height: 0 }, { width: 800, height: 600 }), 1)
})

test("zoom steps by 1.25 within 10% to 800%", () => {
  assert.equal(zoomIn(1), 1.25)
  assert.equal(zoomOut(1), 0.8)
  assert.equal(zoomIn(0.8), 1)
  assert.equal(zoomIn(7), maximumZoom)
  assert.equal(zoomIn(maximumZoom), maximumZoom)
  assert.equal(zoomOut(0.11), minimumZoom)
  assert.equal(zoomOut(minimumZoom), minimumZoom)
  assert.equal(zoomOut(0.05), 0.05)
  assert.equal(zoomIn(0.05), minimumZoom)
  assert.equal(zoomPercent(0.6409), 64)
})
