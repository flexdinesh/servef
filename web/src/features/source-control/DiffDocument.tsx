import {
  Virtualizer, VirtualizedFileDiff,
  type FileDiff as FileDiffInstance, type FileDiffMetadata, type LineDiffTypes, type SelectedLineRange, type SelectionSide,
} from "@pierre/diffs"
import {
  FileDiff, VirtualizerContext, WorkerPoolContextProvider, useWorkerPool,
  type FileDiffOptions, type WorkerInitializationRenderOptions, type WorkerPoolOptions,
} from "@pierre/diffs/react"
import workerUrl from "@pierre/diffs/worker/worker.js?worker&url"
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react"

import { Button } from "@/components/ui/button"
import { Notice } from "@/components/ui/notice"

import { useSettings } from "../../settings/SettingsProvider.tsx"
import { useTheme } from "../../settings/ThemeProvider.tsx"
import { binaryChangeLabel, type DiffContent } from "./diff-content.ts"
import { adjacentHunk, gapForLine, hunkCursor, hunkSelection, type HunkCursor } from "./diff-hunks.ts"
import { diffLineAnchor } from "./diff-line-selection.ts"
import { diffMetrics } from "./diff-metrics.ts"
import { effectiveDiffStyle, type DiffOptions, type InlineChanges } from "./diff-options.ts"
import { DIFF_THEME } from "./diff-theme.ts"
import { formatBytes } from "../../lib/format-bytes.ts"
import { MAIN_THREAD_MAX_CHARACTERS } from "../../highlighting/highlight-thread.ts"
import { uiScale } from "../../settings/settings.ts"
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
  // onRegisterHunkStep hands the toolbar the next/previous change command
  // while a text diff is shown, and null when it is gone.
  onRegisterHunkStep(step: HunkStep | null): void
  onOpenFile(): void
  onRetry(): void
}

// A HunkStep selects and reveals the next (1) or previous (-1) hunk.
export type HunkStep = (direction: 1 | -1) => void

// revealTop is where a revealed line lands: the pane's top third, as in Source.
const revealTop = 1 / 3

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
  content, fileDiff, narrow, options, parseFailed, path, scrollRoot, selection, state, unchangedSince = "since the last commit",
  onRegisterHunkStep, onOpenFile, onRetry,
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
        <Notice layout="bar" className="document-notice" role="note"><span>Renamed from <code>{diff.oldPath}</code></span></Notice>
      )}
      {content.kind === "binary" && (
        <Notice tone="warning" layout="bar" className="document-notice" role="note">{binaryChangeLabel(content.before, content.after)}</Notice>
      )}
      {content.kind === "tooLarge" && (
        <Notice tone="warning" layout="bar" className="document-notice" role="note">Large file ({formatBytes(content.size)}): diff is off.</Notice>
      )}
      {content.kind === "text" && parseFailed && (
        <Notice tone="error" layout="bar" className="document-notice">Couldn't compute this diff.</Notice>
      )}
      {fileDiff && fileDiff.hunks.length === 0 && (
        <Notice layout="bar" className="document-notice" role="note">
          {options.ignoreWhitespace ? "No changes apart from whitespace." : "File contents are unchanged."}
        </Notice>
      )}
    </>
  )
  return (
    <div className="source-stack diff-stack">
      {notices}
      {fileDiff && fileDiff.hunks.length > 0 && <DiffView fileDiff={fileDiff} narrow={narrow} options={options} scrollRoot={scrollRoot}
        selection={selection} onRegisterHunkStep={onRegisterHunkStep} />}
    </div>
  )
}

interface DiffViewProps {
  fileDiff: FileDiffMetadata
  narrow: boolean
  options: DiffOptions
  scrollRoot: RefObject<HTMLElement | null>
  selection: DiffLineSelection
  onRegisterHunkStep(step: HunkStep | null): void
}

type DiffInstance = VirtualizedFileDiff<undefined, undefined>

// expandToLine opens the collapsed gap holding a line, whole.
function expandToLine(instance: DiffInstance, fileDiff: FileDiffMetadata, side: SelectionSide, line: number) {
  const gap = gapForLine(fileDiff, side, line)
  if (gap && !instance.isLineRenderable(gap.newLine)) instance.expandHunk(gap.hunkIndex, "up", gap.lines)
}

// lineTop is a line's offset in the pane's scroll content.
function lineTop(instance: DiffInstance, side: SelectionSide, line: number): { top: number; height: number } | null {
  const position = instance.getLinePosition(line, side)
  if (!position || instance.top === undefined) return null
  return { top: instance.top + position.top, height: position.height }
}

// DiffView renders only the rows near the pane's viewport.
function DiffView({ fileDiff, narrow, options, scrollRoot, selection, onRegisterHunkStep }: DiffViewProps) {
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
  const style = effectiveDiffStyle(options, narrow)
  // Pierre hands over its virtualized instance, which places and expands rows.
  const [instance, setInstance] = useState<DiffInstance | null>(null)
  const onInstance = useCallback((next: FileDiffInstance<undefined, undefined> | null) => {
    setInstance(next instanceof VirtualizedFileDiff ? next : null)
  }, [])
  const anchor = selection.range ? diffLineAnchor(selection.range) : ""
  // A gutter selection is already where the pointer is; only anchors from
  // links, history, and hunk steps reveal.
  const [pointerAnchor, setPointerAnchor] = useState("")
  // The reveal reads the latest rows without revealing again when they
  // refresh.
  const latest = useRef({ fileDiff, range: selection.range })
  useLayoutEffect(() => { latest.current = { fileDiff, range: selection.range } })
  useEffect(() => {
    const root = scrollRoot.current
    if (!root || !instance || !anchor || anchor === pointerAnchor) return
    const frame = window.requestAnimationFrame(() => {
      const { fileDiff: rows, range } = latest.current
      if (!range) return
      const side = range.side ?? "additions"
      expandToLine(instance, rows, side, range.start)
      expandToLine(instance, rows, range.endSide ?? side, range.end)
      const line = lineTop(instance, side, range.start)
      if (!line) return
      const { scrollTop, clientHeight } = root
      if (line.top >= scrollTop && line.top + line.height <= scrollTop + clientHeight) return
      // The pane scrolls itself: the virtualizer clamps to a cached height.
      root.scrollTop = line.top - clientHeight * revealTop
    })
    return () => window.cancelAnimationFrame(frame)
  }, [anchor, instance, pointerAnchor, scrollRoot])
  // Hunk steps start from the selection, else from the pane's top.
  const stepHunk = useCallback((direction: 1 | -1) => {
    if (!instance) return
    const range = selection.range
    let cursor: HunkCursor = { kind: "before", index: fileDiff.hunks.length }
    if (range) {
      cursor = hunkCursor(fileDiff, range.side ?? "additions", range.start)
    } else {
      const scrollTop = virtualizer.getScrollTop()
      const below = fileDiff.hunks.findIndex((_, index) => {
        const first = hunkSelection(fileDiff, index, style)
        const line = first ? lineTop(instance, first.side ?? "additions", first.start) : null
        return line !== null && line.top >= scrollTop
      })
      if (below >= 0) cursor = { kind: "before", index: below }
    }
    const index = adjacentHunk(fileDiff.hunks.length, cursor, direction)
    const next = index === null ? null : hunkSelection(fileDiff, index, style)
    if (!next) return
    setPointerAnchor("")
    onChange(next)
  }, [fileDiff, instance, onChange, selection.range, style, virtualizer])
  useLayoutEffect(() => {
    onRegisterHunkStep(stepHunk)
    return () => onRegisterHunkStep(null)
  }, [onRegisterHunkStep, stepHunk])
  // F7 and Shift+F7 step through hunks while focus is in the pane, as in
  // VS Code's diff editor.
  useEffect(() => {
    const root = scrollRoot.current
    if (!root) return
    const step = (event: KeyboardEvent) => {
      if (event.key !== "F7" || event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return
      event.preventDefault()
      stepHunk(event.shiftKey ? -1 : 1)
    }
    root.addEventListener("keydown", step)
    return () => root.removeEventListener("keydown", step)
  }, [scrollRoot, stepHunk])
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
    diffStyle: style,
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
    // Only pointer gestures end a selection; programmatic ones commit only.
    onLineSelectionEnd(range) {
      setPointerAnchor(range ? diffLineAnchor(range) : "")
    },
  }), [inline, onChange, options, resolvedTheme, scrollRoot, style])
  return (
    <VirtualizerContext.Provider value={virtualizer}>
      <FileDiff key={metrics.lineHeight} className="diff-view" fileDiff={fileDiff} options={diffOptions} metrics={metrics}
        selectedLines={selection.range} onInstance={onInstance}
        renderGutterUtility={() => selection.range && selection.reference
          ? <span className="diff-line-copy"><LineCopyButton reference={selection.reference} /></span>
          : null} />
    </VirtualizerContext.Provider>
  )
}
