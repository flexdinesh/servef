import type { TreeNode } from "./page-contract.ts"

// Filename filters mirror the CLI's --filename semantics: comma-separated Go
// path.Match globs, matched case-insensitively against the file base name.

type Token =
  | { kind: "star" }
  | { kind: "any" }
  | { kind: "literal"; char: string }
  | { kind: "class"; negated: boolean; ranges: ReadonlyArray<readonly [string, string]> }

export interface FilenameMatcher {
  matches(name: string): boolean
}

export type FilenameFilterResult =
  | { valid: true; matcher: FilenameMatcher | null }
  | { valid: false; pattern: string }

class BadPattern extends Error {}

// foldCase mirrors Go strings.ToLower: simple per-rune mapping, so no
// context-sensitive final sigma and no multi-rune expansions. U+0130 is the
// only unconditional multi-rune lowercase; Go maps it to "i".
function foldCase(value: string): string {
  return Array.from(value, (char) => {
    if (char === "\u0130") return "i"
    const lower = char.toLowerCase()
    return Array.from(lower).length === 1 ? lower : char
  }).join("")
}

function compileGlob(pattern: string): Token[] {
  const chars = Array.from(pattern)
  const tokens: Token[] = []
  let index = 0
  // Reads one class member, honoring escapes; Go rejects bare "-", "]", and an unterminated class.
  const member = (): string => {
    let char = chars[index]
    if (char === undefined || char === "-" || char === "]") throw new BadPattern()
    if (char === "\\") {
      index++
      char = chars[index]
      if (char === undefined) throw new BadPattern()
    }
    index++
    if (index >= chars.length) throw new BadPattern()
    return char
  }
  while (index < chars.length) {
    const char = chars[index] ?? ""
    if (char === "*") {
      if (tokens.at(-1)?.kind !== "star") tokens.push({ kind: "star" })
      index++
    } else if (char === "?") {
      tokens.push({ kind: "any" })
      index++
    } else if (char === "\\") {
      const escaped = chars[index + 1]
      if (escaped === undefined) throw new BadPattern()
      tokens.push({ kind: "literal", char: escaped })
      index += 2
    } else if (char === "[") {
      index++
      const negated = chars[index] === "^"
      if (negated) index++
      const ranges: Array<readonly [string, string]> = []
      while (!(chars[index] === "]" && ranges.length > 0)) {
        const lo = member()
        let hi = lo
        if (chars[index] === "-") {
          index++
          hi = member()
        }
        ranges.push([lo, hi])
      }
      index++
      tokens.push({ kind: "class", negated, ranges })
    } else {
      tokens.push({ kind: "literal", char })
      index++
    }
  }
  return tokens
}

function matchesChar(token: Token, char: string): boolean {
  if (token.kind === "any") return char !== "/"
  if (token.kind === "literal") return token.char === char
  if (token.kind === "class") {
    const point = char.codePointAt(0) ?? 0
    const inRange = token.ranges.some(([lo, hi]) => (lo.codePointAt(0) ?? 0) <= point && point <= (hi.codePointAt(0) ?? 0))
    return inRange !== token.negated
  }
  return false
}

function matchTokens(tokens: readonly Token[], name: string): boolean {
  const chars = Array.from(name)
  let tokenIndex = 0
  let charIndex = 0
  let starToken = -1
  let starChar = 0
  while (charIndex < chars.length) {
    const token = tokens[tokenIndex]
    const char = chars[charIndex] ?? ""
    if (token?.kind === "star") {
      starToken = tokenIndex++
      starChar = charIndex
    } else if (token && matchesChar(token, char)) {
      tokenIndex++
      charIndex++
    } else if (starToken >= 0 && chars[starChar] !== "/") {
      tokenIndex = starToken + 1
      charIndex = ++starChar
    } else {
      return false
    }
  }
  while (tokens[tokenIndex]?.kind === "star") tokenIndex++
  return tokenIndex === tokens.length
}

export function parseFilenameFilter(input: string): FilenameFilterResult {
  const globs: Token[][] = []
  // Empty entries are ignored so partially typed lists such as "*.md," stay usable.
  for (const entry of input.split(",")) {
    const pattern = entry.trim()
    if (!pattern) continue
    try {
      globs.push(compileGlob(foldCase(pattern)))
    } catch (error) {
      if (error instanceof BadPattern) return { valid: false, pattern }
      throw error
    }
  }
  if (globs.length === 0) return { valid: true, matcher: null }
  return {
    valid: true,
    matcher: { matches: (name) => globs.some((tokens) => matchTokens(tokens, foldCase(name))) },
  }
}

// Tree visibility is independent of the server's file scope. Preserve original
// node metadata and prune directories without visible descendants.
export function filterTreeByFilename(nodes: readonly TreeNode[], matcher: FilenameMatcher | null): TreeNode[] {
  if (!matcher) return [...nodes]
  return nodes.flatMap((node) => {
    if (!node.isDir) return matcher.matches(node.name) ? [node] : []
    const children = filterTreeByFilename(node.children, matcher)
    return children.length ? [{ ...node, children }] : []
  })
}

export function countFiles(nodes: readonly TreeNode[]): number {
  return nodes.reduce((total, node) => total + (node.isDir ? countFiles(node.children) : 1), 0)
}

export function isDefaultFilenameScope(patterns: readonly string[]): boolean {
  return patterns.length === 0 || (patterns.length === 1 && patterns[0] === "*")
}
