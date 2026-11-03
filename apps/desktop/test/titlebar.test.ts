import assert from "node:assert/strict"
import { test } from "node:test"
import { parseMenuPoint, parseThemeColor, symbolColorFor, TITLEBAR_HEIGHT, windowChrome } from "../src/main/titlebar.ts"

test("windowChrome hides the title bar and colors overlay controls outside macOS", () => {
  const platforms: NodeJS.Platform[] = ["linux", "win32"]
  for (const platform of platforms) {
    assert.deepEqual(windowChrome(platform, "#0b0b0b"), {
      titleBarStyle: "hidden",
      titleBarOverlay: { color: "#0b0b0b", symbolColor: "#e6e6e6", height: TITLEBAR_HEIGHT },
      backgroundColor: "#0b0b0b",
    })
  }
})

test("windowChrome keeps macOS traffic lights centered in the band", () => {
  const chrome = windowChrome("darwin", "#ffffff")
  assert.equal(chrome.titleBarStyle, "hidden")
  assert.equal(chrome.titleBarOverlay, true)
  assert.equal(chrome.backgroundColor, "#ffffff")
  assert.deepEqual(chrome.trafficLightPosition, { x: 12, y: (TITLEBAR_HEIGHT - 16) / 2 })
})

test("parseThemeColor accepts only opaque #rrggbb", () => {
  assert.equal(parseThemeColor("#0B0B0B"), "#0b0b0b")
  assert.equal(parseThemeColor("#f7f8fa"), "#f7f8fa")
  for (const color of [null, "", "#fff", "#ffffff80", "rgb(0, 0, 0)", "red", "#gggggg", " #ffffff", "#ffffff;", "url(x)"]) {
    assert.equal(parseThemeColor(color), null, String(color))
  }
})

test("symbolColorFor contrasts with the band", () => {
  // Dark canvases (Servef Dark, Tokyo Night) get light controls.
  for (const color of ["#000000", "#0b0b0b", "#16161e", "#3a3a3a"]) assert.equal(symbolColorFor(color), "#e6e6e6", color)
  // Light canvases (Servef Light, Solarized Light) get dark controls.
  for (const color of ["#ffffff", "#f7f8fa", "#eee8d5", "#a0a0a0"]) assert.equal(symbolColorFor(color), "#3b3b3b", color)
})

test("parseMenuPoint accepts finite points inside the window", () => {
  const size = { width: 800, height: 600 }
  assert.deepEqual(parseMenuPoint({ x: 4.4, y: 32 }, size), { x: 4, y: 32 })
  assert.deepEqual(parseMenuPoint({ x: 0, y: 0 }, size), { x: 0, y: 0 })
  assert.deepEqual(parseMenuPoint({ x: 800, y: 600 }, size), { x: 800, y: 600 })
  for (const point of [null, undefined, 3, "1,2", [1, 2], {}, { x: 1 }, { x: "1", y: 2 }, { x: Number.NaN, y: 2 }, { x: Infinity, y: 2 }, { x: -1, y: 2 }, { x: 1, y: 601 }, { x: 801, y: 0 }]) {
    assert.equal(parseMenuPoint(point, size), null, JSON.stringify(point))
  }
})
