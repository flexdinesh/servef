import { Virtualizer, type FileDiffMetadata, type LineDiffTypes, type SelectedLineRange } from "@pierre/diffs"
import {
  FileDiff, VirtualizerContext, WorkerPoolContextProvider, useWorkerPool,
  type FileDiffOptions, type WorkerInitializationRenderOptions, type WorkerPoolOptions,
} from "@pierre/diffs/react"
import workerUrl from "@pierre/diffs/worker/worker.js?worker&url"
import { useEffect, useLayoutEffect, useMemo, useState, type RefObject } from "react"

import { Button } from "@/components/ui/button"

import { useSettings } from "../../components/SettingsProvider.tsx"
import { useTheme } from "../../components/ThemeProvider.tsx"
import { binaryChangeLabel, type DiffContent } from "./diff-content.ts"
import { diffMetrics } from "./diff-metrics.ts"
import { effectiveDiffStyle, type DiffOptions, type InlineChanges } from "./diff-options.ts"
import { DIFF_THEME } from "./diff-theme.ts"
import { formatBytes } from "../../format-bytes.ts"
import { MAIN_THREAD_MAX_CHARACTERS } from "../../highlighting/highlight-thread.ts"
import { uiScale } from "../../settings.ts"
import { LineCopyButton } from "../documents/SourceDocument.tsx"
import type { DiffState } from "./use-working-tree-diff.ts"

// One servef theme serves both polarities; themeType picks the scheme.
const theme = { light: DIFF_THEME, dark: DIFF_THEME }

// Highlights tokenizes small diffs on the main thread; larger ones go to two
// locally bundled workers while a diff is shown (decision 0029).
const poolOptions: WorkerPoolOptions = {
  workerFactory: () => new Worker(workerUrl, { type: "module" }),
  poolSize: 2,
  mainThreadMaxCharacters: MAIN_THREAD_MAX_CHARACTERS,
}

// Browser surfaces inside Pierre's shadow root follow servef tokens.
const diffCSS = `::selection { color: var(--color-text-primary); background: var(--color-accent-subtle); }`

// DiffLineSelection connects gutter selection to its owner (the URL fragment).
export interface DiffLineSelection {
  range: SelectedLineRange | null
  // reference is the copied path:lines, empty when the range addresses nothing.
  reference: string
  onChange(range: SelectedLineRange | null): void
}

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
  selection: DiffLineSelection
  state: DiffState
  // unchangedSince ends the “No changes to path …” line: “since the last
  // commit” for working-tree diffs, “vs main” for branch diffs.
  unchangedSince?: string
  onOpenFile(): void
  onRetry(): void
}

// DiffDocument keeps one worker pool while any diff tab is shown, across
// switches between diff tabs.
export function DiffDocument(props: DiffDocumentProps) {
  // The pool reads these once; later changes go through setRenderOptions.
  const [highlighterOptions] = useState((): WorkerInitializationRenderOptions => ({
    theme, lineDiffType: lineDiffType(props.options.inline), preferredHighlighter: "highlights",
  }))
  return (
    <WorkerPoolContextProvider poolOptions={poolOptions} highlighterOptions={highlighterOptions}>
      <DiffBody {...props} />
    </WorkerPoolContextProvider>
  )
}

function DiffBody({
  content, fileDiff, narrow, options, parseFailed, path, scrollRoot, selection, state, unchangedSince = "since the last commit", onOpenFile, onRetry,
}: DiffDocumentProps) {
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
        <p>No changes to <code>{path}</code> {unchangedSince}.</p>
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
      {fileDiff && fileDiff.hunks.length > 0 && <DiffView fileDiff={fileDiff} narrow={narrow} options={options} scrollRoot={scrollRoot}
        selection={selection} />}
    </div>
  )
}

interface DiffViewProps {
  fileDiff: FileDiffMetadata
  narrow: boolean
  options: DiffOptions
  scrollRoot: RefObject<HTMLElement | null>
  selection: DiffLineSelection
}

// DiffView renders only the rows near the pane's viewport.
function DiffView({ fileDiff, narrow, options, scrollRoot, selection }: DiffViewProps) {
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
  // Escape clears the selection while focus is in the pane, as in Source.
  const hasSelection = selection.range !== null
  const { onChange } = selection
  useEffect(() => {
    const root = scrollRoot.current
    if (!root || !hasSelection) return
    const clear = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return
      event.preventDefault()
      onChange(null)
    }
    root.addEventListener("keydown", clear)
    return () => root.removeEventListener("keydown", clear)
  }, [hasSelection, onChange, scrollRoot])
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
    preferredHighlighter: "highlights",
    diffStyle: effectiveDiffStyle(options, narrow),
    overflow: options.wrap ? "wrap" : "scroll",
    lineDiffType: inline,
    expandUnchanged: options.expandUnchanged,
    disableFileHeader: true,
    unsafeCSS: diffCSS,
    // Line numbers select lines: click, drag, or Shift-click to extend.
    enableLineSelection: true,
    // The gutter utility holds the copy button at the selection's last line.
    enableGutterUtility: true,
    onLineSelected(range) {
      // Selection moves focus to the pane so Escape clears it.
      scrollRoot.current?.focus({ preventScroll: true })
      onChange(range)
    },
  }), [inline, narrow, onChange, options, resolvedTheme, scrollRoot])
  return (
    <VirtualizerContext.Provider value={virtualizer}>
      <FileDiff key={metrics.lineHeight} className="diff-view" fileDiff={fileDiff} options={diffOptions} metrics={metrics}
        selectedLines={selection.range}
        renderGutterUtility={() => selection.range && selection.reference
          ? <span className="diff-line-copy"><LineCopyButton reference={selection.reference} /></span>
          : null} />
    </VirtualizerContext.Provider>
  )
}
