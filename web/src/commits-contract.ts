import { isChangeStatus, type ChangeStatus } from "./changes-contract.ts"
import { isCommitHash } from "./workspace-tabs.ts"

// Transport contracts for /api/commits and /api/commit-files (decision 0027).

// CommitRange is base..HEAD ("branch") or HEAD's whole history ("all").
export type CommitRange = "branch" | "all"

export interface Commit {
  hash: string
  parents: string[]
  subject: string
  // body is the message after the subject line, "" when there is none.
  body: string
  author: { name: string; email: string }
  // authorTime is RFC 3339 with the author's offset.
  authorTime: string
}

export interface CommitsResponse {
  // repository is false for a folder project; commits are then empty.
  repository: boolean
  branch: string
  // base is the branch the range compares with, "" when none was found.
  base: string
  range: CommitRange
  // head is the commit the walk started from; later pages pass it back.
  head: string
  // ahead counts base..head, 0 without a base.
  ahead: number
  commits: Commit[]
  more: boolean
}

// CommitFileStatus never includes "untracked": commits hold tracked files only.
export type CommitFileStatus = Exclude<ChangeStatus, "untracked">

export interface CommitFile {
  path: string
  oldPath?: string
  status: CommitFileStatus
}

export interface CommitFilesResponse {
  commit: string
  files: CommitFile[]
  truncated: boolean
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

export function isCommitRange(value: unknown): value is CommitRange {
  return value === "branch" || value === "all"
}

function isCommit(value: unknown): value is Commit {
  return isRecord(value)
    && isCommitHash(value.hash)
    && Array.isArray(value.parents)
    && value.parents.every(isCommitHash)
    && typeof value.subject === "string"
    && typeof value.body === "string"
    && isRecord(value.author)
    && typeof value.author.name === "string"
    && typeof value.author.email === "string"
    && typeof value.authorTime === "string"
    && !Number.isNaN(Date.parse(value.authorTime))
}

export function isCommitsResponse(value: unknown): value is CommitsResponse {
  return isRecord(value)
    && typeof value.repository === "boolean"
    && typeof value.branch === "string"
    && typeof value.base === "string"
    && isCommitRange(value.range)
    && (value.head === "" || isCommitHash(value.head))
    && typeof value.ahead === "number"
    && Number.isInteger(value.ahead)
    && value.ahead >= 0
    && Array.isArray(value.commits)
    && value.commits.every(isCommit)
    && (value.repository || value.commits.length === 0)
    && typeof value.more === "boolean"
}

function isCommitFile(value: unknown): value is CommitFile {
  return isRecord(value)
    && typeof value.path === "string"
    && value.path !== ""
    && isChangeStatus(value.status)
    && value.status !== "untracked"
    && (value.oldPath === undefined || typeof value.oldPath === "string")
    && (value.status !== "renamed" || typeof value.oldPath === "string")
}

export function isCommitFilesResponse(value: unknown): value is CommitFilesResponse {
  return isRecord(value)
    && isCommitHash(value.commit)
    && Array.isArray(value.files)
    && value.files.every(isCommitFile)
    && typeof value.truncated === "boolean"
}

const historyErrorPrefix = "Couldn't read git history: "

// historyError phrases a failure as "Couldn't read git history: <reason>.",
// keeping a server message that already says so.
export function historyError(reason: string): string {
  const message = reason.startsWith(historyErrorPrefix) ? reason : `${historyErrorPrefix}${reason}`
  return /[.!?]$/.test(message) ? message : `${message}.`
}
