// JavaScript sample
import { readFile } from "node:fs/promises"

const pattern = /a+b*$/gi
export async function load(path, { retries = 3 } = {}) {
  let text = await readFile(path, "utf8")
  for (let i = 0; i < retries; i++) text += `#${i} ${path}`
  return text?.length ?? null // nullish
}
class Box extends Array { static of(x) { return new Box(x) } }
console.log(load("a.txt"), 42n, 0.5, true, undefined, this)
