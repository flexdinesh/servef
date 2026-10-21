import { Virtualizer, type FileDiffMetadata, type LineDiffTypes } from "@pierre/diffs"
import {
  FileDiff, VirtualizerContext, WorkerPoolContextProvider, useWorkerPool,
  type FileDiffOptions, type WorkerInitializationRenderOptions, type WorkerPoolOptions,
} from "@pierre/diffs/react"
import workerUrl from "@pierre/diffs/worker/worker.js?worker&url"
import { useEffect, useLayoutEffect, useMemo, useState, type RefObject } from "react"

import { Button } from "@/components/ui/button"

import { useSettings } from "./components/SettingsProvider.tsx"
import { useTheme } from "./components/ThemeProvider.tsx"
import { binaryChangeLabel, type DiffContent } from "./diff-content.ts"
import { diffMetrics } from "./diff-metrics.ts"
import { effectiveDiffStyle, type DiffOptions, type InlineChanges } from "./diff-options.ts"
import { DIFF_THEME } from "./diff-theme.ts"
import { formatBytes } from "./format-bytes.ts"
import { uiScale } from "./settings.ts"
import type { DiffState } from "./use-working-tree-diff.ts"

// One servef theme serves both polarities; themeType picks the scheme.
const theme = { light: DIFF_THEME, dark: DIFF_THEME }

// Highlighting runs in two locally bundled workers while a diff is shown.
const poolOptions: WorkerPoolOptions = {
  workerFactory: () => new Worker(workerUrl, { type: "module" }),
  poolSize: 2,
}

// Browser surfaces inside Pierre's shadow root follow servef tokens.
const diffCSS = `::selection { color: var(--color-text-primary); background: var(--color-accent-subtle); }`

function lineDiffType(inline: InlineChanges): LineDiffTypes {
  return inline === "word" ? "word-alt" : inline
}

interface DiffDocumentProps {
  content: DiffContent | null
  // fileDiff is the parsed text diff, null when content is not text or
  // while the first parse runs.
  fileDiff: FileDiffMetadata | null
  // narrow forces Unified, as Split has no room.
  narrow: boolean
  options: DiffOptions
  // parseFailed: the text sides could not be diffed.
  parseFailed: boolean
  path: string
  // scrollRoot is the document pane, which scrolls the virtualized diff.
  scrollRoot: RefObject<HTMLElement | null>
  state: DiffState
  onOpenFile(): void
  onRetry(): void
}

// DiffDocument keeps one worker pool while any diff tab is shown, across
// switches between diff tabs.
export function DiffDocument(props: DiffDocumentProps) {
  // The pool reads these once; later changes go through setRenderOptions.
  const [highlighterOptions] = useState((): WorkerInitializationRenderOptions => ({ theme, lineDiffType: lineDiffType(props.options.inline) }))
  return (
    <WorkerPoolContextProvider poolOptions={poolOptions} highlighterOptions={highlighterOptions}>
      <DiffBody {...props} />
    </WorkerPoolContextProvider>
  )
}

function DiffBody({ content, fileDiff, narrow, options, parseFailed, path, scrollRoot, state, onOpenFile, onRetry }: DiffDocumentProps) {
  if (state.kind === "error") {
    return (
      <div className="error" role="alert">
        <p>{state.message}</p>
        <Button type="button" variant="outline" onClick={onRetry}>Retry</Button>
      </div>
    )
  }
  if (state.kind === "unchanged") {
    return (
      <div className="empty">
        <h1>No changes</h1>
        <p>No changes to <code>{path}</code> since the last commit.</p>
        <Button type="button" variant="outline" onClick={onOpenFile}>Open File</Button>
      </div>
    )
  }
  if (state.kind === "loading" || !content) return null
  const { diff } = state
  const notices = (
    <>
      {diff.status === "renamed" && diff.oldPath && (
        <p className="document-notice document-notice-info" role="note">Renamed from <code>{diff.oldPath}</code></p>
      )}
      {content.kind === "binary" && (
        <p className="document-notice document-notice-warning" role="note">{binaryChangeLabel(content.before, content.after)}</p>
      )}
      {content.kind === "tooLarge" && (
        <p className="document-notice document-notice-warning" role="note">Large file ({formatBytes(content.size)}): diff is off.</p>
      )}
      {content.kind === "text" && parseFailed && (
        <p className="document-notice document-notice-error" role="alert">Couldn't compute this diff.</p>
      )}
      {fileDiff && fileDiff.hunks.length === 0 && (
        <p className="document-notice document-notice-info" role="note">
          {options.ignoreWhitespace ? "No changes apart from whitespace." : "File contents are unchanged."}
        </p>
      )}
    </>
  )
  return (
    <div className="source-stack diff-stack">
      {notices}
      {fileDiff && fileDiff.hunks.length > 0 && <DiffView fileDiff={fileDiff} narrow={narrow} options={options} scrollRoot={scrollRoot} />}
    </div>
  )
}

interface DiffViewProps {
  fileDiff: FileDiffMetadata
  narrow: boolean
  options: DiffOptions
  scrollRoot: RefObject<HTMLElement | null>
}

// DiffView renders only the rows near the pane's viewport.
function DiffView({ fileDiff, narrow, options, scrollRoot }: DiffViewProps) {
  const { resolvedTheme } = useTheme()
  const pool = useWorkerPool()
  const [virtualizer] = useState(() => new Virtualizer())
  // The pane, not a nested scroller, scrolls, so keyboard scrolling and
  // focus keep working. Children connect before this setup runs.
  useLayoutEffect(() => {
    const root = scrollRoot.current
    if (!root) return
    virtualizer.setup(root)
    return () => virtualizer.cleanUp()
  }, [scrollRoot, virtualizer])
  // Pierre reads row metrics once, so a Font Size change remounts the diff.
  const scale = uiScale(useSettings().settings)
  const metrics = useMemo(() => diffMetrics(scale), [scale])
  const inline = lineDiffType(options.inline)
  // Workers render with pool-wide options, so inline emphasis goes there.
  useEffect(() => {
    pool?.setRenderOptions({ theme, lineDiffType: inline }).catch(() => {
      // A stopped pool falls back to main-thread highlighting.
    })
  }, [inline, pool])
  const diffOptions = useMemo((): FileDiffOptions<undefined, undefined> => ({
    theme,
    themeType: resolvedTheme,
    diffStyle: effectiveDiffStyle(options, narrow),
    overflow: options.wrap ? "wrap" : "scroll",
    lineDiffType: inline,
    expandUnchanged: options.expandUnchanged,
    disableFileHeader: true,
    unsafeCSS: diffCSS,
  }), [inline, narrow, options, resolvedTheme])
  return (
    <VirtualizerContext.Provider value={virtualizer}>
      <FileDiff key={metrics.lineHeight} className="diff-view" fileDiff={fileDiff} options={diffOptions} metrics={metrics} />
    </VirtualizerContext.Provider>
  )
}
