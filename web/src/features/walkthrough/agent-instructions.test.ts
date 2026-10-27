import assert from "node:assert/strict"
import { readFileSync } from "node:fs"
import test from "node:test"

import { agentInstructions } from "./agent-instructions.ts"

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
}

const schema: unknown = JSON.parse(readFileSync(new URL("../../../../internal/walkthrough/schema.json", import.meta.url), "utf8"))

function resolve(node: unknown): Record<string, unknown> {
  assert.ok(isRecord(node) && isRecord(schema) && isRecord(schema.$defs))
  if (typeof node.$ref !== "string") return node
  const definition = schema.$defs[node.$ref.replace("#/$defs/", "")]
  assert.ok(isRecord(definition), `unknown ${node.$ref}`)
  return definition
}

// problems checks value against the subset of JSON Schema schema.json uses,
// so the copied example cannot name a field or value the server rejects.
function problems(value: unknown, node: unknown, at: string): string[] {
  const rule = resolve(node)
  const found: string[] = []
  if (Array.isArray(rule.enum) && !rule.enum.includes(value)) found.push(`${at}: not one of ${rule.enum.join(", ")}`)
  if (rule.type === "string") {
    if (typeof value !== "string") return [`${at}: not a string`]
    if (typeof rule.pattern === "string" && !new RegExp(rule.pattern).test(value)) found.push(`${at}: does not match ${rule.pattern}`)
  }
  if (rule.type === "boolean" && typeof value !== "boolean") found.push(`${at}: not a boolean`)
  if (rule.type === "integer" && !Number.isInteger(value)) found.push(`${at}: not an integer`)
  if (rule.type === "array") {
    if (!Array.isArray(value)) return [`${at}: not an array`]
    value.forEach((item, index) => {
      const itemRule = Array.isArray(rule.prefixItems) ? rule.prefixItems[index] : rule.items
      found.push(...problems(item, itemRule, `${at}[${index}]`))
    })
  }
  if (rule.type === "object") {
    if (!isRecord(value)) return [`${at}: not an object`]
    const properties = isRecord(rule.properties) ? rule.properties : {}
    for (const [key, item] of Object.entries(value)) {
      if (key in properties) found.push(...problems(item, properties[key], `${at}.${key}`))
      else found.push(`${at}.${key}: unknown field`)
    }
    for (const key of Array.isArray(rule.required) ? rule.required : []) {
      if (typeof key === "string" && !(key in value)) found.push(`${at}.${key}: missing`)
    }
  }
  return found
}

function example(prompt: string): unknown {
  const match = /```json\n([\s\S]*?)\n```/.exec(prompt)
  assert.ok(match?.[1], "the prompt carries a JSON example")
  return JSON.parse(match[1])
}

test("instructions name the running servef, the project, and the endpoints", () => {
  const prompt = agentInstructions("http://localhost:7971", "/home/me/src/app")
  assert.match(prompt, /^servef: http:\/\/localhost:7971$/m)
  assert.match(prompt, /^Project: \/home\/me\/src\/app$/m)
  assert.match(prompt, /POST http:\/\/localhost:7971\/api\/walkthroughs \(Content-Type: application\/json\)/)
  assert.match(prompt, /GET http:\/\/localhost:7971\/api\/walkthrough-schema/)
  assert.match(prompt, /--data @walkthrough\.json http:\/\/localhost:7971\/api\/walkthroughs$/m)
  assert.ok(prompt.endsWith("\n\nWalk me through: "))
})

test("the example walkthrough follows the server's schema", () => {
  const walkthrough = example(agentInstructions("http://localhost:7971", "/home/me/src/app"))
  assert.deepEqual(problems(walkthrough, schema, "walkthrough"), [])
  assert.ok(isRecord(walkthrough) && Array.isArray(walkthrough.steps))
  const targets = walkthrough.steps.map((step) => isRecord(step) ? Object.keys(step).filter((key) => key === "file" || key === "diff").join() : "")
  assert.deepEqual(new Set(targets), new Set(["", "file", "diff"]))
  assert.deepEqual(problems({ ...walkthrough, steps: [{ title: "x", file: { path: "a", line: 1 } }] }, schema, "walkthrough"), [
    "walkthrough.steps[0].file.line: unknown field",
  ])
})

test("the example quotes awkward project paths as JSON", () => {
  const project = String.raw`C:\Users\me\"quoted" app`
  const walkthrough = example(agentInstructions("http://127.0.0.1:7972", project))
  assert.ok(isRecord(walkthrough))
  assert.equal(walkthrough.project, project)
})
