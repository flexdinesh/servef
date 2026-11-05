import assert from "node:assert/strict"
import { test } from "node:test"
import { MAX_ITEM_ID, MAX_ITEM_LABEL, MAX_MENU_ITEMS, parseContextMenuRequest } from "../src/main/context-menu.ts"

const size = { width: 800, height: 600 }

test("parses items, separators, enabled, and accelerator hints at a point", () => {
  const request = parseContextMenuRequest({
    items: [
      { id: "close", label: "Close", accelerator: "CmdOrCtrl+W" },
      { separator: true },
      { id: "reveal", label: "Reveal in File Manager", enabled: false },
    ],
    x: 10.4,
    y: 20.6,
  }, size)
  assert.deepEqual(request, {
    items: [
      { kind: "item", id: "close", label: "Close", enabled: true, accelerator: "CmdOrCtrl+W" },
      { kind: "separator" },
      { kind: "item", id: "reveal", label: "Reveal in File Manager", enabled: false, accelerator: null },
    ],
    point: { x: 10, y: 21 },
  })
})

test("drops leading, trailing, and repeated separators", () => {
  const request = parseContextMenuRequest({
    items: [{ separator: true }, { id: "a", label: "A" }, { separator: true }, { separator: true }, { id: "b", label: "B" }, { separator: true }],
    x: 0,
    y: 0,
  }, size)
  assert.deepEqual(request?.items.map((item) => item.kind === "item" ? item.id : "-"), ["a", "-", "b"])
})

test("refuses a menu with any invalid item", () => {
  const valid = { id: "ok", label: "OK" }
  const invalid: unknown[] = [
    null,
    "close",
    { id: "", label: "Empty id" },
    { id: "has space", label: "X" },
    { id: "x".repeat(MAX_ITEM_ID + 1), label: "Long id" },
    { id: 1, label: "Number id" },
    { id: "x", label: "" },
    { id: "x", label: "   " },
    { id: "x", label: "y".repeat(MAX_ITEM_LABEL + 1) },
    { id: "x", label: "line\nbreak" },
    { id: "x", label: "nul\u0000" },
    { id: "x", label: "X", enabled: "yes" },
    { id: "x", label: "X", accelerator: "Ctrl+" },
    { id: "x", label: "X", accelerator: "Hyper+K" },
    { id: "x", label: "X", accelerator: "Ctrl+K extra" },
    { id: "x", label: "X", accelerator: 5 },
    { separator: true, id: "x" },
    { separator: "yes" },
  ]
  for (const item of invalid) {
    assert.equal(parseContextMenuRequest({ items: [valid, item], x: 1, y: 1 }, size), null, JSON.stringify(item))
  }
  assert.equal(parseContextMenuRequest({ items: [valid, { ...valid }], x: 1, y: 1 }, size), null, "duplicate ids")
})

test("refuses empty, oversized, and separator-only menus", () => {
  assert.equal(parseContextMenuRequest({ items: [], x: 1, y: 1 }, size), null)
  assert.equal(parseContextMenuRequest({ items: [{ separator: true }, { separator: true }], x: 1, y: 1 }, size), null)
  const many = Array.from({ length: MAX_MENU_ITEMS + 1 }, (_, index) => ({ id: `i${index}`, label: `Item ${index}` }))
  assert.equal(parseContextMenuRequest({ items: many, x: 1, y: 1 }, size), null)
  assert.notEqual(parseContextMenuRequest({ items: many.slice(0, MAX_MENU_ITEMS), x: 1, y: 1 }, size), null)
  assert.equal(parseContextMenuRequest({ items: "menu", x: 1, y: 1 }, size), null)
  assert.equal(parseContextMenuRequest([], size), null)
})

test("refuses points outside the window or not finite", () => {
  const items = [{ id: "a", label: "A" }]
  for (const [x, y] of [[-1, 0], [0, -1], [801, 0], [0, 601], [Number.NaN, 0], [0, Number.POSITIVE_INFINITY]]) {
    assert.equal(parseContextMenuRequest({ items, x, y }, size), null, `${x},${y}`)
  }
  assert.equal(parseContextMenuRequest({ items, x: "1", y: 1 }, size), null)
  assert.equal(parseContextMenuRequest({ items }, size), null)
  assert.notEqual(parseContextMenuRequest({ items, x: 800, y: 600 }, size), null)
})

test("accepts accelerators in Electron's grammar", () => {
  for (const accelerator of ["CmdOrCtrl+Shift+P", "Alt+F4", "Delete", "Shift+F10", "Ctrl+,", "Ctrl+Plus", "F24"]) {
    assert.notEqual(parseContextMenuRequest({ items: [{ id: "a", label: "A", accelerator }], x: 0, y: 0 }, size), null, accelerator)
  }
})
