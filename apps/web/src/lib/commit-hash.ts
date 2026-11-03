// isCommitHash accepts a full, lowercase SHA-1 commit hash.
export function isCommitHash(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{40}$/.test(value)
}

// shortHash abbreviates a commit hash as git log --oneline does.
export function shortHash(hash: string): string {
  return hash.slice(0, 7)
}
