// Search scopes follow VS Code's "files to include/exclude": comma-separated,
// case-insensitive path globs. "*" and "?" stay within a segment, "**" spans
// segments, "{a,b}" alternates, and "[...]" is a class. A pattern without a
// leading "./" matches at any depth, and a matched folder covers its contents.

export interface PathMatcher {
  matches(path: string): boolean
}

export type PathScopeResult =
  | { valid: true; matcher: PathMatcher | null }
  | { valid: false; pattern: string }

class BadPattern extends Error {}

function escapeRegExp(char: string): string {
  return /[.*+?^${}()|[\]\\/]/.test(char) ? `\\${char}` : char
}

// splitTopLevel splits on commas outside braces so "*.{ts,tsx}" stays whole.
function splitTopLevel(input: string): string[] {
  const parts: string[] = []
  let depth = 0
  let current = ""
  for (const char of input) {
    if (char === "{") depth++
    else if (char === "}" && depth > 0) depth--
    if (char === "," && depth === 0) {
      parts.push(current)
      current = ""
    } else {
      current += char
    }
  }
  parts.push(current)
  return parts
}

function compileBody(pattern: string): string {
  const chars = Array.from(pattern)
  let source = ""
  let index = 0
  while (index < chars.length) {
    const char = chars[index] ?? ""
    if (char === "*") {
      if (chars[index + 1] === "*") {
        index += 2
        if (chars[index] === "/") {
          index++
          source += "(?:.*/)?"
        } else {
          source += ".*"
        }
      } else {
        index++
        source += "[^/]*"
      }
    } else if (char === "?") {
      index++
      source += "[^/]"
    } else if (char === "[") {
      const close = chars.indexOf("]", index + 2)
      if (close < 0) throw new BadPattern()
      let members = chars.slice(index + 1, close)
      const negated = members[0] === "!" || members[0] === "^"
      if (negated) members = members.slice(1)
      source += `[${negated ? "^" : ""}${members.map((member) => member === "\\" || member === "]" || member === "[" ? `\\${member}` : member).join("")}]`
      index = close + 1
    } else if (char === "{") {
      let depth = 0
      let close = -1
      for (let cursor = index; cursor < chars.length; cursor++) {
        if (chars[cursor] === "{") depth++
        else if (chars[cursor] === "}" && --depth === 0) {
          close = cursor
          break
        }
      }
      if (close < 0) throw new BadPattern()
      const alternatives = splitTopLevel(chars.slice(index + 1, close).join(""))
      source += `(?:${alternatives.map(compileBody).join("|")})`
      index = close + 1
    } else if (char === "\\") {
      const escaped = chars[index + 1]
      if (escaped === undefined) throw new BadPattern()
      source += escapeRegExp(escaped)
      index += 2
    } else {
      source += escapeRegExp(char)
      index++
    }
  }
  return source
}

function compilePattern(pattern: string): RegExp {
  let body = pattern.replaceAll("\\", "/").replace(/\/+$/, "")
  let anchored = false
  if (body.startsWith("./")) {
    anchored = true
    body = body.slice(2)
  } else if (body.startsWith("/")) {
    anchored = true
    body = body.slice(1)
  }
  if (!body) throw new BadPattern()
  const prefix = anchored || body.startsWith("**") ? "" : "(?:.*/)?"
  try {
    return new RegExp(`^${prefix}${compileBody(body)}(?:/.*)?$`, "iu")
  } catch (error) {
    if (error instanceof SyntaxError) throw new BadPattern()
    throw error
  }
}

export function parsePathScope(input: string): PathScopeResult {
  const patterns: RegExp[] = []
  for (const entry of splitTopLevel(input)) {
    const pattern = entry.trim()
    if (!pattern) continue
    try {
      patterns.push(compilePattern(pattern))
    } catch (error) {
      if (error instanceof BadPattern) return { valid: false, pattern }
      throw error
    }
  }
  if (patterns.length === 0) return { valid: true, matcher: null }
  return { valid: true, matcher: { matches: (path) => patterns.some((pattern) => pattern.test(path)) } }
}
