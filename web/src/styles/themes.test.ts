import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

import { themeOptions } from "../settings/theme.ts"

const themesCSS = readFileSync(new URL("themes.css", import.meta.url), "utf8")
const tokensCSS = readFileSync(new URL("tokens.css", import.meta.url), "utf8")

// Theme values are the colors and shadows tokens.css declares in @theme.
const themeBlock = /@theme\s*\{([^}]*)\}/.exec(tokensCSS)?.[1] ?? ""
const themeValues = [...themeBlock.matchAll(/(--(?:color|shadow)-[a-z-]+):/g)].map((match) => match[1])

function namedThemes(): Map<string, Map<string, string>> {
  const themes = new Map<string, Map<string, string>>()
  for (const [, name, body] of themesCSS.matchAll(/:root\[data-theme="([^"]+)"\]\s*\{([^}]*)\}/g)) {
    themes.set(name, new Map([...body.matchAll(/(--[a-z-]+):\s*([^;]+);/g)].map(([, key, value]) => [key, value.trim()])))
  }
  return themes
}

// Light and Dark are the two sides of each light-dark() color in @theme.
function defaultThemes(): Map<string, Map<string, string>> {
  const light = new Map<string, string>()
  const dark = new Map<string, string>()
  for (const [, key, value] of themeBlock.matchAll(/(--color-[a-z-]+):\s*([^;]+);/g)) {
    const pair = /^light-dark\(([^,]+),\s*(.+)\)$/.exec(value.trim())
    light.set(key, pair ? pair[1].trim() : value.trim())
    dark.set(key, pair ? pair[2].trim() : value.trim())
  }
  return new Map([["light", light], ["dark", dark]])
}

function allThemes(): Map<string, Map<string, string>> {
  return new Map([...defaultThemes(), ...namedThemes()])
}

function channels(hex: string): [number, number, number] {
  const match = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex)
  assert.ok(match, `${hex} is a 6-digit hex color`)
  return [Number.parseInt(match[1], 16) / 255, Number.parseInt(match[2], 16) / 255, Number.parseInt(match[3], 16) / 255]
}

function luminance(hex: string): number {
  const [r, g, b] = channels(hex).map((value) => value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

function contrast(a: string, b: string): number {
  const [light, dark] = [luminance(a), luminance(b)].toSorted((x, y) => y - x)
  return (light + 0.05) / (dark + 0.05)
}

function hue(hex: string): number {
  const [r, g, b] = channels(hex)
  const max = Math.max(r, g, b)
  const delta = max - Math.min(r, g, b)
  if (delta === 0) return 0
  const sector = max === r ? ((g - b) / delta) % 6 : max === g ? (b - r) / delta + 2 : (r - g) / delta + 4
  return (sector * 60 + 360) % 360
}

const textRoles = [
  "text-primary", "text-secondary", "text-muted", "accent", "success", "warning", "destructive",
  "syntax-keyword", "syntax-type", "syntax-function", "syntax-variable", "syntax-string", "syntax-number", "syntax-operator",
]

test("every named theme has exactly one themes.css block", () => {
  const named = themeOptions.map((option) => option.value).filter((value) => value !== "system" && value !== "light" && value !== "dark")
  assert.deepEqual([...namedThemes().keys()].toSorted(), named.toSorted())
})

test("named themes override every theme value", () => {
  assert.ok(themeValues.length > 0)
  for (const [name, values] of namedThemes()) {
    assert.deepEqual([...values.keys()].toSorted(), themeValues.toSorted(), name)
  }
})

test("every theme keeps text, status, links, and code readable", () => {
  for (const [name, values] of allThemes()) {
    const value = (key: string) => values.get(`--color-${key}`) ?? ""
    for (const ground of ["surface", "surface-secondary"]) {
      for (const role of textRoles) {
        const ratio = contrast(value(role), value(ground))
        assert.ok(ratio >= 4.5, `${name}: ${role} on ${ground} is ${ratio.toFixed(2)}:1`)
      }
      const comment = contrast(value("syntax-comment"), value(ground))
      assert.ok(comment >= 3, `${name}: syntax-comment on ${ground} is ${comment.toFixed(2)}:1`)
    }
    const onAccent = contrast(value("on-accent"), value("accent"))
    assert.ok(onAccent >= 4.5, `${name}: on-accent on accent is ${onAccent.toFixed(2)}:1`)
    const selected = contrast(value("accent-hover"), value("accent-subtle"))
    assert.ok(selected >= 4.5, `${name}: accent-hover on accent-subtle is ${selected.toFixed(2)}:1`)
  }
})

test("every theme keeps additions and deletions distinct hues", () => {
  for (const [name, values] of allThemes()) {
    const distance = Math.abs(hue(values.get("--color-success") ?? "") - hue(values.get("--color-destructive") ?? ""))
    assert.ok(Math.min(distance, 360 - distance) >= 60, `${name}: success and destructive hues are ${distance.toFixed(0)}° apart`)
  }
})
