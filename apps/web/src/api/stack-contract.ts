import { isRecord } from "./http.ts"
import { isCommitHash } from "../lib/commit-hash.ts"

// Transport contract for /api/branch-stack (decision 0040): the local
// branches stacked between the base branch and HEAD, and those built on top
// of HEAD. /api/branch-files, /api/diff?branch=1, and
// /api/compare-stats?compare=branch accept a layer as head=<name>; a name the
// repository does not list as a local branch is a 400 with code
// unknownHeadCode.

export const unknownHeadCode = "unknownHead"

export interface StackLayer {
  // name is the layer's local branch, "" for a detached HEAD.
  name: string
  // aliases are other local branches on the same commit.
  aliases: string[]
  head: string
  // parent is the layer below, or the base branch for the lowest layer.
  parent: string
  // ahead counts the layer's own commits, those its parent does not have.
  ahead: number
  // current marks HEAD's layer.
  current: boolean
}

export interface StackResponse {
  // repository is false for a folder project; layers are then empty.
  repository: boolean
  // branch is the checked-out branch, "" when HEAD is detached.
  branch: string
  // base is "" when no base branch was found.
  base: string
  // mergeBase is "" while nothing is ahead or there is no base.
  mergeBase: string
  // head is "" when HEAD is unborn.
  head: string
  // layers run from the top of the stack down to the layer nearest the base;
  // empty while HEAD has nothing ahead of the base.
  layers: StackLayer[]
  truncated: boolean
}

function isOptionalHash(value: unknown): boolean {
  return value === "" || isCommitHash(value)
}

function isStackLayer(value: unknown): value is StackLayer {
  return isRecord(value)
    && typeof value.name === "string"
    && Array.isArray(value.aliases)
    && value.aliases.every((alias) => typeof alias === "string" && alias !== "")
    && isCommitHash(value.head)
    && typeof value.parent === "string"
    && typeof value.ahead === "number"
    && Number.isInteger(value.ahead)
    && value.ahead >= 0
    && typeof value.current === "boolean"
    && (value.name !== "" || value.current)
}

export function isStackResponse(value: unknown): value is StackResponse {
  return isRecord(value)
    && typeof value.repository === "boolean"
    && typeof value.branch === "string"
    && typeof value.base === "string"
    && isOptionalHash(value.mergeBase)
    && isOptionalHash(value.head)
    && Array.isArray(value.layers)
    && value.layers.every(isStackLayer)
    && (value.repository || value.layers.length === 0)
    && typeof value.truncated === "boolean"
}
