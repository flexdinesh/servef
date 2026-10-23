import assert from "node:assert/strict"
import test from "node:test"

import { defaultDiffOptions, effectiveDiffStyle, parseDiffOptions, type DiffOptions } from "./diff-options.ts"

test("stored diff options keep valid values and default the rest", () => {
  assert.deepEqual(parseDiffOptions(null), defaultDiffOptions)
  assert.deepEqual(parseDiffOptions(["unified"]), defaultDiffOptions)
  assert.deepEqual(parseDiffOptions({ style: "unified", wrap: true, ignoreWhitespace: true, inline: "char", expandUnchanged: true }),
    { style: "unified", wrap: true, ignoreWhitespace: true, inline: "char", expandUnchanged: true })
  assert.deepEqual(parseDiffOptions({ style: "stacked", wrap: "yes", inline: "line", expandUnchanged: 1 }), defaultDiffOptions)
  assert.deepEqual(parseDiffOptions({ inline: "none" }), { ...defaultDiffOptions, inline: "none" })
})

test("narrow windows force Unified without forgetting Split", () => {
  const split: DiffOptions = { ...defaultDiffOptions, style: "split" }
  assert.equal(effectiveDiffStyle(split, false), "split")
  assert.equal(effectiveDiffStyle(split, true), "unified")
  assert.equal(effectiveDiffStyle({ ...split, style: "unified" }, false), "unified")
})
