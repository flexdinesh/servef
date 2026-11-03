import { isRecord } from "./http.ts"

// Transport contract for /api/refs: the branches a compare may use as its
// base. /api/commits, /api/branch-files, and /api/diff?branch=1 accept one as
// base=<name>; a name the repository does not list is a 400 with code
// unknownBaseCode.

export const unknownBaseCode = "unknownBase"

export interface BranchRef {
  // name is git's short form: "main" for a local branch, "origin/main" for a
  // remote-tracking one.
  name: string
  remote: boolean
}

export interface RefsResponse {
  // repository is false for a folder project; branches are then empty.
  repository: boolean
  // branch is the checked-out branch, "" when HEAD is detached.
  branch: string
  // defaultBase is the base chosen when none is given, "" when none exists.
  defaultBase: string
  // branches lists local branches, then remote-tracking ones, each by name.
  branches: BranchRef[]
  truncated: boolean
}

function isBranchRef(value: unknown): value is BranchRef {
  return isRecord(value)
    && typeof value.name === "string"
    && value.name !== ""
    && typeof value.remote === "boolean"
}

export function isRefsResponse(value: unknown): value is RefsResponse {
  return isRecord(value)
    && typeof value.repository === "boolean"
    && typeof value.branch === "string"
    && typeof value.defaultBase === "string"
    && Array.isArray(value.branches)
    && value.branches.every(isBranchRef)
    && (value.repository || value.branches.length === 0)
    && typeof value.truncated === "boolean"
}
