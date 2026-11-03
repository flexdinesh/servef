// Source Control failure phrasing and the error codes the server's
// contract.Code* constants name (pkg/contract/codes.go, Go-only).

// unknownBaseCode marks a 400 for a base branch the repository does not list
// (/api/commits, /api/branch-files, /api/diff?branch=1, /api/branch-stack).
export const unknownBaseCode = "unknownBase"

// unknownHeadCode marks a 400 for a head that is not a local branch
// (/api/branch-files, /api/diff?branch=1, /api/compare-stats?compare=branch).
export const unknownHeadCode = "unknownHead"

const statusErrorPrefix = "Couldn't read git status: "
const historyErrorPrefix = "Couldn't read git history: "

// gitStatusError phrases a failure as "Couldn't read git status: <reason>.",
// keeping a server message that already says so.
export function gitStatusError(reason: string): string {
  return phrased(statusErrorPrefix, reason)
}

// historyError phrases a failure as "Couldn't read git history: <reason>.",
// keeping a server message that already says so.
export function historyError(reason: string): string {
  return phrased(historyErrorPrefix, reason)
}

function phrased(prefix: string, reason: string): string {
  const message = reason.startsWith(prefix) ? reason : `${prefix}${reason}`
  return /[.!?]$/.test(message) ? message : `${message}.`
}
