import assert from "node:assert/strict"
import test from "node:test"

import { RenderError } from "ondine/compiler/wasm"

import {
  clampDiagramZoom,
  fitDiagramView,
  mermaidErrorMessage,
  mermaidRenderError,
  panDiagramView,
  zoomDiagramView,
} from "./diagram.ts"
import { maximumZoom, minimumZoom } from "./image-zoom.ts"

test("zoom clamps to bounds and recovers from invalid values", () => {
  assert.equal(clampDiagramZoom(100), maximumZoom)
  assert.equal(clampDiagramZoom(0), minimumZoom)
  assert.equal(clampDiagramZoom(Number.NaN), 1)
  assert.equal(clampDiagramZoom(1.5), 1.5)
})

test("fit keeps small diagrams at natural size and centers them", () => {
  assert.deepEqual(fitDiagramView({ width: 800, height: 600 }, { width: 200, height: 100 }), { x: 300, y: 250, zoom: 1 })
})

test("fit scales large diagrams down to the viewport", () => {
  const view = fitDiagramView({ width: 400, height: 400 }, { width: 1600, height: 400 })

  assert.equal(view.zoom, 0.25)
  assert.equal(view.x, 0)
  assert.equal(view.y, 150)
})

test("fit falls back to natural size for empty sizes", () => {
  assert.equal(fitDiagramView({ width: 0, height: 0 }, { width: 100, height: 100 }).zoom, 1)
  assert.equal(fitDiagramView({ width: 100, height: 100 }, { width: 0, height: 0 }).zoom, 1)
})

test("zoom keeps the anchor point fixed", () => {
  const view = { x: 10, y: 20, zoom: 1 }
  const anchor = { x: 110, y: 70 }
  const zoomed = zoomDiagramView(view, 2, anchor)
  const before = { x: (anchor.x - view.x) / view.zoom, y: (anchor.y - view.y) / view.zoom }
  const after = { x: (anchor.x - zoomed.x) / zoomed.zoom, y: (anchor.y - zoomed.y) / zoomed.zoom }

  assert.equal(zoomed.zoom, 2)
  assert.deepEqual(after, before)
})

test("zoom at a bound leaves the view unchanged", () => {
  const view = { x: 5, y: 5, zoom: maximumZoom }

  assert.deepEqual(zoomDiagramView(view, 2, { x: 50, y: 50 }), view)
})

test("pan moves the view without zooming", () => {
  assert.deepEqual(panDiagramView({ x: 1, y: 2, zoom: 3 }, 10, -5), { x: 11, y: -3, zoom: 3 })
})

test("render errors keep ondine's diagnostic with line and snippet", () => {
  const message = "Parse error on line 3:\n    B --> [oops\n----------^\nExpecting 'NODE_STRING', got 'SQS'"
  const error = new RenderError({ code: "syntax", column: 11, line: 3, message, snippet: "    B --> [oops\n----------^", type: "flowchart" })

  assert.equal(mermaidErrorMessage(error), message)
})

test("other failures get the generic render message", () => {
  assert.equal(mermaidErrorMessage(new Error("unreachable")), mermaidRenderError)
  assert.equal(mermaidErrorMessage("boom"), mermaidRenderError)
})
