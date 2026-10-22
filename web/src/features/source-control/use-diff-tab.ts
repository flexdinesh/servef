import { useNavigate } from "@tanstack/react-router"
import { useCallback, useMemo, type RefObject } from "react"

import type { SelectedLineRange } from "@pierre/diffs"
import { diffContent } from "./diff-content.ts"
import { diffLineAnchor, diffLineTarget, parseDiffLineAnchor } from "./diff-line-selection.ts"
import { effectiveDiffStyle } from "./diff-options.ts"
import { lineReference } from "../../lib/line-selection.ts"
import { filePaths, type TreeNode } from "../../api/page-data.ts"
import type { Changes } from "./use-changes.ts"
import { useDelayedFlag } from "../../lib/use-delayed-flag.ts"
import { useDiffOptions } from "./use-diff-options.ts"
import { useParsedDiff } from "./use-parsed-diff.ts"
import { useWorkingTreeDiff } from "./use-working-tree-diff.ts"
import { sameTab, tabKey, tabSearch, type WorkspaceTab } from "../../workspace/workspace-tabs.ts"

interface DiffTabOptions {
  project: string
  // tab is the routed diff tab of any kind, null on a document.
  tab: WorkspaceTab | null
  changes: Changes
  // tree is the project's file tree; a history diff opens the working copy
  // only while the tree still has it.
  tree: readonly TreeNode[]
  // hash is the route fragment, which holds a diff's line selection.
  hash: string
  // narrow forces Unified diffs.
  narrow: boolean
  main: RefObject<HTMLElement | null>
  prepareDocumentNavigation(): void
}

// useDiffTab owns the open diff tab: its sides, parsed hunks, toolbar
// options, line selection, and opening diff tabs from Source Control. A
// history diff's rename is resolved by the server from git itself.
export function useDiffTab({ project, tab, changes, tree, hash, narrow, main, prepareDocumentNavigation }: DiffTabOptions) {
  const navigate = useNavigate()
  const path = tab?.path ?? ""
  // working is set for a working-tree diff tab; history diffs (commit,
  // branch, range) read git objects only.
  const working = tab?.kind === "diff"
  const options = useDiffOptions()
  const changedFile = working ? changes.byPath.get(path) : undefined
  const read = useWorkingTreeDiff(project, tab, changedFile?.oldPath ?? "")
  const state = read.state
  const content = useMemo(() => state.kind === "ready" ? diffContent(state.diff) : null, [state])
  const parsed = useParsedDiff(tab ? tabKey(tab) : "", content, options.options.ignoreWhitespace)
  const { fileDiff } = parsed
  const slow = useDelayedFlag(read.loading || parsed.parsing, 150)
  const deleted = (state.kind === "ready" ? state.diff.status : changedFile?.status) === "deleted"
  const isDeleted = useCallback((candidate: string) => changes.byPath.get(candidate)?.status === "deleted"
    || (candidate === path && working && deleted), [changes.byPath, deleted, path, working])
  const treeFiles = useMemo(() => new Set(filePaths(tree)), [tree])
  const openFileReason = tab && !working
    ? treeFiles.has(path) ? "" : "This file isn't in the working tree"
    : deleted ? "This file was deleted; there's no working copy to open" : ""

  // openDiff opens a change's diff tab. Click keeps focus in Source Control;
  // Enter and double-click move it to the diff, as in Search.
  const openDiff = useCallback((next: string, focusDocument: boolean) => {
    if (next === path && working) {
      if (focusDocument) main.current?.focus({ preventScroll: true })
      return
    }
    if (focusDocument) prepareDocumentNavigation()
    void navigate({ to: "/view", search: { project, path: next, diff: 1 }, hash: "" })
  }, [main, navigate, path, prepareDocumentNavigation, project, working])
  // openHistoryDiff opens a commit, branch, or range file's diff tab,
  // focusing as openDiff does.
  const openHistoryDiff = useCallback((next: WorkspaceTab, focusDocument: boolean) => {
    if (sameTab(next, tab)) {
      if (focusDocument) main.current?.focus({ preventScroll: true })
      return
    }
    if (focusDocument) prepareDocumentNavigation()
    void navigate({ to: "/view", search: tabSearch(project, next), hash: "" })
  }, [main, navigate, prepareDocumentNavigation, project, tab])
  const activeHistory = tab && !working ? tab : null

  // A diff's line selection lives in its fragment too: #R12-R20, #L5.
  const selection = path ? parseDiffLineAnchor(hash) : null
  const target = fileDiff && selection ? diffLineTarget(fileDiff, selection, effectiveDiffStyle(options.options, narrow)) : null
  const oldPath = state.kind === "ready" && state.diff.oldPath ? state.diff.oldPath : path
  const reference = target ? lineReference(target.side === "old" ? oldPath : path, target.range) : ""
  const selectLines = useCallback((range: SelectedLineRange | null) => {
    if (!tab) return
    void navigate({ to: "/view", search: tabSearch(project, tab), hash: range ? diffLineAnchor(range) : "", replace: true })
  }, [navigate, project, tab])

  return {
    path, working, activeHistory, options, state, content, fileDiff, parseFailed: parsed.failed, retry: read.retry, slow,
    isDeleted, openFileReason, openDiff, openHistoryDiff,
    // target is the selected lines resolved against the parsed diff.
    target,
    selection: { range: target ? selection : null, reference, onChange: selectLines },
  }
}
