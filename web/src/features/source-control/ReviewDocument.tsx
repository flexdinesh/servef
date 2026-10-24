import {
  Virtualizer, VirtualizedFileDiff,
  type FileDiff as FileDiffInstance, type FileDiffMetadata, type LineDiffTypes, type SelectedLineRange, type SelectionSide,
} from "@pierre/diffs"
import {
  FileDiff, VirtualizerContext, WorkerPoolContextProvider, useWorkerPool,
  type FileDiffOptions, type WorkerInitializationRenderOptions, type WorkerPoolOptions,
} from "@pierre/diffs/react"
import workerUrl from "@pierre/diffs/worker/worker.js?worker&url"
import { CheckIcon, FileInputIcon } from "lucide-react"
import {
  createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode, type RefObject,
} from "react"

import { Button } from "@/components/ui/button"
import { Notice, StatusMessage } from "@/components/ui/notice"

import { FileIcon } from "../../components/FileIcon.tsx"
import { Chevron } from "../../components/ui/section-toggle.tsx"
import { IconButton } from "../../components/ui/icon-button.tsx"
import { ToggleButton } from "../../components/ui/toggle-button.tsx"
import type { FileStats } from "../../api/compare-stats-contract.ts"
import { formatBytes } from "../../lib/format-bytes.ts"
import { shortHash } from "../../lib/commit-hash.ts"
import { lineReference } from "../../lib/line-selection.ts"
import { MAIN_THREAD_MAX_CHARACTERS } from "../../highlighting/highlight-thread.ts"
import { useSettings } from "../../settings/SettingsProvider.tsx"
import { useTheme } from "../../settings/ThemeProvider.tsx"
import { uiScale } from "../../settings/settings.ts"
import { LineCopyButton } from "../documents/SourceDocument.tsx"
import { commitDate, type Compare, type CompareName } from "./compare.ts"
import { displayOrder, type CompareFile } from "./compare-files.ts"
import { binaryChangeLabel } from "./diff-content.ts"
import { fileOrder, gapForLine, hunkCursor, hunkInFile, hunkSelection, type HunkCursor } from "./diff-hunks.ts"
import { diffLineAnchor, diffLineTarget, type DiffLineTarget } from "./diff-line-selection.ts"
import { diffMetrics } from "./diff-metrics.ts"
import { effectiveDiffStyle, type DiffOptions, type InlineChanges } from "./diff-options.ts"
import { diffStatsLabel } from "./diff-stats.ts"
import { DIFF_THEME } from "./diff-theme.ts"
import type { CollapseRequest } from "./FilesTree.tsx"
import { loadReviewDiff, useReviewDiff } from "./review-diffs.ts"
import type { CompareData } from "./use-compare-data.ts"
import { statsLabel, totals } from "./use-compare-stats.ts"

// One servef theme serves both polarities; themeType picks the scheme.
const theme = { light: DIFF_THEME, dark: DIFF_THEME }

// Highlights tokenizes small diffs on the main thread; larger ones go to two
// locally bundled workers while a review is shown (decision 0029).
const poolOptions: WorkerPoolOptions = {
  workerFactory: () => new Worker(workerUrl, { type: "module" }),
  poolSize: 2,
  mainThreadMaxCharacters: MAIN_THREAD_MAX_CHARACTERS,
}

// Browser surfaces inside Pierre's shadow root follow servef tokens.
const diffCSS = `::selection { color: var(--color-text-primary); background: var(--color-accent-subtle); }`

// Sections within this distance of the viewport read and render their diffs.
const nearMargin = "1600px 0px"

// revealTop is where a revealed line lands: the pane's top third, as in Source.
const revealTop = 1 / 3

const statusLetters: Record<CompareFile["status"], string> = { modified: "M", added: "A", untracked: "U", deleted: "D", renamed: "R" }
const statusNames: Record<CompareFile["status"], string> = {
  modified: "Modified", added: "Added", untracked: "Untracked", deleted: "Deleted", renamed: "Renamed",
}

function lineDiffType(inline: InlineChanges): LineDiffTypes {
  return inline === "word" ? "word-alt" : inline
}

// ReviewSelection is the selected lines and the file they belong to.
export interface ReviewSelection {
  path: string
  lines: SelectedLineRange | null
}

// ReviewStatus is what the status bar shows for the file in view.
export interface ReviewStatus {
  path: string
  target: DiffLineTarget | null
  typeLabel: string
}

interface ReviewDocumentProps {
  project: string
  compare: Compare
  name: CompareName
  data: CompareData | null
  options: DiffOptions
  narrow: boolean
  // scrollRoot is the document pane, which scrolls every section.
  scrollRoot: RefObject<HTMLElement | null>
  // selection comes from the fragment: the file to show and its lines.
  selection: ReviewSelection | null
  // expandRequest opens or closes every file's diff; id repeats a request.
  expandRequest: CollapseRequest | null
  // treeFiles are the project's files; Open File needs a working copy.
  treeFiles: ReadonlySet<string>
  viewed(path: string, blob: string): boolean
  onViewedChange(path: string, blob: string, viewed: boolean): void
  // onSelectLines replaces the fragment; null lines keep only the file.
  onSelectLines(path: string, lines: SelectedLineRange | null): void
  onCurrentFile(path: string): void
  onStatus(status: ReviewStatus | null): void
  onOpenFile(path: string): void
  // onAllExpandedChange reports whether any file's diff is open.
  onAllExpandedChange(expanded: boolean): void
  // onRegisterHunkStep hands the toolbar the next/previous change command,
  // and null when the review is gone.
  onRegisterHunkStep(step: HunkStep | null): void
}

// A HunkStep selects and reveals the next (1) or previous (-1) change,
// crossing into other files (decisions 0034, 0035).
export type HunkStep = (direction: 1 | -1) => void

// A ShownDiff is a rendered file diff, which places and expands its rows.
interface ShownDiff {
  fileDiff: FileDiffMetadata
  // lineBox is a line's box in the pane's scroll content, null when unplaced.
  lineBox(side: SelectionSide, line: number): { top: number; height: number } | null
  // expandTo opens the collapsed gap holding a line, whole.
  expandTo(side: SelectionSide, line: number): void
}

// ShownContext lets rendered diffs register by path for reveals and steps.
type RegisterShown = (path: string, shown: ShownDiff | null) => void
const ShownContext = createContext<RegisterShown>(() => {})

// ObserveContext shares one IntersectionObserver across sections.
type Observe = (element: Element, onNear: (near: boolean) => void) => () => void
const ObserveContext = createContext<Observe>(() => () => {})

// ReviewDocument stacks a compare's file diffs in one scroll (decision 0035),
// in the Files tree's order. Sections read their diff as they near the
// viewport and let go of it far away.
export function ReviewDocument(props: ReviewDocumentProps) {
  // The pool reads these once; later changes go through setRenderOptions.
  const [highlighterOptions] = useState((): WorkerInitializationRenderOptions => ({
    theme, lineDiffType: lineDiffType(props.options.inline), preferredHighlighter: "highlights",
  }))
  return (
    <WorkerPoolContextProvider poolOptions={poolOptions} highlighterOptions={highlighterOptions}>
      <ReviewBody {...props} />
    </WorkerPoolContextProvider>
  )
}

function ReviewBody({
  compare, data, expandRequest, name, narrow, options, project, scrollRoot, selection, treeFiles,
  onAllExpandedChange, onCurrentFile, onOpenFile, onRegisterHunkStep, onSelectLines, onStatus, onViewedChange, viewed,
}: ReviewDocumentProps) {
  const pool = useWorkerPool()
  const [virtualizer] = useState(() => new Virtualizer())
  const container = useRef<HTMLDivElement>(null)
  const list = data?.list
  const files = useMemo(() => list?.kind === "ready" ? displayOrder(list.files) : [], [list])
  const stats = data?.stats ?? null
  // open overrides a file's default: viewed files start closed, others open.
  const [open, setOpen] = useState<ReadonlyMap<string, boolean>>(() => new Map())
  const isOpen = useCallback((file: CompareFile) => {
    const blob = stats?.byPath.get(file.path)?.blob ?? ""
    return open.get(file.path) ?? !viewed(file.path, blob)
  }, [open, stats, viewed])

  // The pane, not a nested scroller, scrolls, so keyboard scrolling and focus
  // keep working. A fresh load mounts the pane with this review, and layout
  // effects run before the pane's ref is set, so set up in a passive effect;
  // the virtualizer queues children that connect first.
  useEffect(() => {
    const root = scrollRoot.current
    if (!root) return
    virtualizer.setup(root)
    return () => virtualizer.cleanUp()
  }, [scrollRoot, virtualizer])

  // Workers render with pool-wide options, so inline emphasis goes there.
  const inline = lineDiffType(options.inline)
  useEffect(() => {
    pool?.setRenderOptions({ theme, lineDiffType: inline }).catch(() => {
      // A stopped pool falls back to main-thread highlighting.
    })
  }, [inline, pool])

  // One observer tells sections when they near the viewport.
  const observers = useRef(new Map<Element, (near: boolean) => void>())
  const [observer, setObserver] = useState<IntersectionObserver | null>(null)
  // Passive, like the virtualizer's setup, so the pane's ref is set.
  useEffect(() => {
    const root = scrollRoot.current
    if (!root) return
    const next = new IntersectionObserver((entries) => {
      for (const entry of entries) observers.current.get(entry.target)?.(entry.isIntersecting)
    }, { root, rootMargin: nearMargin })
    setObserver(next)
    return () => next.disconnect()
  }, [scrollRoot])
  const observe = useCallback<Observe>((element, onNear) => {
    if (!observer) return () => {}
    observers.current.set(element, onNear)
    observer.observe(element)
    return () => {
      observers.current.delete(element)
      observer.unobserve(element)
    }
  }, [observer])

  // Expand or Collapse All sets every file once per request.
  const [applied, setApplied] = useState(expandRequest?.id ?? 0)
  if (expandRequest && expandRequest.id !== applied) {
    setApplied(expandRequest.id)
    setOpen(new Map(files.map((file): [string, boolean] => [file.path, expandRequest.expand])))
  }
  const anyOpen = files.some(isOpen)
  useEffect(() => onAllExpandedChange(anyOpen), [anyOpen, onAllExpandedChange])

  // Rendered diffs, by path; a pin repins when the pinned file's diff shows.
  const shown = useRef(new Map<string, ShownDiff>())
  const repin = useRef<{ path: string; pin(): void } | null>(null)
  const registerShown = useCallback<RegisterShown>((path, next) => {
    if (next) shown.current.set(path, next)
    else shown.current.delete(path)
    if (next && repin.current?.path === path) repin.current.pin()
  }, [])

  // The fragment's file scrolls into view when it changes, and stays pinned
  // there while diffs above it load and grow, until the user scrolls. Its
  // lines, once their diff shows, land near the top third with their
  // collapsed gaps open (decision 0034), unless already in view. While
  // pinned it is the file in view, even when too near the end to reach the
  // top. A gutter selection is where the pointer is, so it never scrolls.
  const target = selection?.path ?? ""
  const anchor = selection?.lines ? diffLineAnchor(selection.lines) : ""
  const latestLines = useRef(selection?.lines ?? null)
  useLayoutEffect(() => { latestLines.current = selection?.lines ?? null })
  // pointer is the last gutter selection as `path#anchor`.
  const pointer = useRef("")
  const ready = files.length > 0
  const pinned = useRef("")
  useEffect(() => {
    const root = scrollRoot.current
    const content = container.current
    if (!target || !ready || !root || !content) return
    const fromPointer = pointer.current === `${target}#${anchor}`
    pointer.current = ""
    if (fromPointer) return
    const lineBox = () => {
      const range = latestLines.current
      const view = shown.current.get(target)
      if (!range || !view) return null
      const side = range.side ?? "additions"
      view.expandTo(side, range.start)
      view.expandTo(range.endSide ?? side, range.end)
      return view.lineBox(side, range.start)
    }
    const line = lineBox()
    if (line && line.top >= root.scrollTop && line.top + line.height <= root.scrollTop + root.clientHeight) return
    const pin = () => {
      const section = content.querySelector(`[data-review-path="${CSS.escape(target)}"]`)
      if (!(section instanceof HTMLElement)) return
      const box = lineBox()
      const top = box ? box.top - root.clientHeight * revealTop
        : section.getBoundingClientRect().top - root.getBoundingClientRect().top + root.scrollTop
      if (Math.abs(root.scrollTop - top) > 1) root.scrollTo({ top })
    }
    pinned.current = target
    repin.current = { path: target, pin }
    pin()
    const observer = new ResizeObserver(pin)
    observer.observe(content)
    const events: ReadonlyArray<keyof HTMLElementEventMap> = ["wheel", "pointerdown", "keydown", "touchstart"]
    const release = () => {
      pinned.current = ""
      repin.current = null
      observer.disconnect()
      for (const type of events) root.removeEventListener(type, onInput)
    }
    // A press inside the content (selecting lines, toggling Viewed) keeps the
    // pin; one on the pane itself is its scrollbar.
    const onInput = (event: Event) => {
      if (event.type !== "pointerdown" || event.target === root) release()
    }
    for (const type of events) root.addEventListener(type, onInput, { passive: true })
    return release
  }, [anchor, ready, scrollRoot, target])

  // The pinned file, else the one whose section holds the pane's top edge, is
  // the one in view.
  const currentFile = useRef("")
  useEffect(() => {
    const root = scrollRoot.current
    if (!root || !ready) return
    let frame = 0
    const update = () => {
      frame = 0
      const top = root.getBoundingClientRect().top + 1
      let path = pinned.current
      for (const section of pinned.current ? [] : container.current?.querySelectorAll<HTMLElement>("[data-review-path]") ?? []) {
        if (section.getBoundingClientRect().top > top) break
        path = section.dataset.reviewPath ?? ""
      }
      path ||= files[0]?.path ?? ""
      if (path !== currentFile.current) {
        currentFile.current = path
        onCurrentFile(path)
      }
    }
    const schedule = () => { if (!frame) frame = requestAnimationFrame(update) }
    update()
    root.addEventListener("scroll", schedule, { passive: true })
    return () => {
      root.removeEventListener("scroll", schedule)
      if (frame) cancelAnimationFrame(frame)
    }
  }, [files, onCurrentFile, ready, scrollRoot, target])

  // Steps walk the open files' changes in order: the next change in the
  // file holding the selection (else the file in view), then the first of
  // the next file with any, wrapping past the ends. A file not yet shown is
  // read first.
  const style = effectiveDiffStyle(options, narrow)
  const stepping = useRef<AbortController | null>(null)
  const latestStep = useRef({
    files, isOpen, selection, style, version: data?.version ?? "", ignoreWhitespace: options.ignoreWhitespace, root: scrollRoot.current,
  })
  useLayoutEffect(() => {
    latestStep.current = {
      files, isOpen, selection, style, version: data?.version ?? "", ignoreWhitespace: options.ignoreWhitespace, root: scrollRoot.current,
    }
  })
  const stepHunk = useCallback<HunkStep>((direction) => {
    stepping.current?.abort()
    const controller = new AbortController()
    stepping.current = controller
    const { files: all, isOpen: opened, selection: selected, style: shownStyle, version, ignoreWhitespace, root } = latestStep.current
    const candidates = all.filter(opened)
    const load = async (file: CompareFile): Promise<FileDiffMetadata | null> => {
      const view = shown.current.get(file.path)
      if (view) return view.fileDiff
      const diff = await loadReviewDiff({ project, compare, file, version, ignoreWhitespace }, controller.signal)
      return diff.kind === "ready" ? diff.fileDiff : null
    }
    // cursor places the step in its first file: at the selection, else below
    // the pane's top.
    const cursor = (path: string, fileDiff: FileDiffMetadata): HunkCursor => {
      const lines = selected?.path === path ? selected.lines : null
      if (lines) return hunkCursor(fileDiff, lines.side ?? "additions", lines.start)
      const view = shown.current.get(path)
      const scrollTop = root?.scrollTop ?? 0
      const below = view ? fileDiff.hunks.findIndex((_, index) => {
        const first = hunkSelection(fileDiff, index, shownStyle)
        const line = first ? view.lineBox(first.side ?? "additions", first.start) : null
        return line !== null && line.top >= scrollTop
      }) : -1
      return { kind: "before", index: below >= 0 ? below : direction === 1 && !view ? 0 : fileDiff.hunks.length }
    }
    const step = async () => {
      const fromPath = selected?.lines ? selected.path : currentFile.current
      const from = candidates.findIndex((file) => file.path === fromPath)
      const start = candidates[from]
      const startDiff = start ? await load(start) : null
      if (start && startDiff) {
        const index = hunkInFile(startDiff.hunks.length, cursor(start.path, startDiff), direction)
        const next = index === null ? null : hunkSelection(startDiff, index, shownStyle)
        if (next) return onSelectLines(start.path, next)
      }
      for (const index of fileOrder(candidates.length, from, direction)) {
        const file = candidates[index]
        const fileDiff = file ? await load(file) : null
        const count = fileDiff?.hunks.length ?? 0
        const next = file && fileDiff && count > 0 ? hunkSelection(fileDiff, direction === 1 ? 0 : count - 1, shownStyle) : null
        if (file && next) return onSelectLines(file.path, next)
      }
    }
    step().catch(() => {
      // Aborted by a newer step or unmount.
    })
  }, [compare, onSelectLines, project])
  useEffect(() => () => stepping.current?.abort(), [])

  useLayoutEffect(() => {
    onRegisterHunkStep(stepHunk)
    return () => onRegisterHunkStep(null)
  }, [onRegisterHunkStep, stepHunk])

  // F7 and Shift+F7 step through changes while focus is in the pane, as in
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

  const sum = totals(stats)
  const viewedCount = files.filter((file) => viewed(file.path, stats?.byPath.get(file.path)?.blob ?? "")).length

  return (
    <ObserveContext.Provider value={observe}>
      <ShownContext.Provider value={registerShown}>
      <VirtualizerContext.Provider value={virtualizer}>
        <div ref={container} className="review">
          <header className="review-header">
            <h1 className="review-title" title={name.title}>{reviewHeading(compare, data, name)}</h1>
            <ReviewMeta compare={compare} data={data} />
            <div className="review-totals">
              {list?.kind === "ready" && <span>{list.files.length.toLocaleString()}{list.truncated ? "+" : ""} {list.files.length === 1 ? "file" : "files"}</span>}
              {sum && <span className="review-totals-lines" title={`${sum.additions.toLocaleString()} added, ${sum.deletions.toLocaleString()} deleted lines`}>
                {diffStatsLabel(sum)}
              </span>}
              {files.length > 0 && <span>{viewedCount.toLocaleString()} of {files.length.toLocaleString()} viewed</span>}
            </div>
          </header>
          {(!list || list.kind === "loading") && <StatusMessage className="review-note">Loading files…</StatusMessage>}
          {list?.kind === "error" && (
            <div className="review-note" role="alert">
              <StatusMessage tone="error">{list.message}</StatusMessage>
              <Button variant="outline" onClick={data?.retry}>Retry</Button>
            </div>
          )}
          {list?.kind === "ready" && list.files.length === 0 && (
            <StatusMessage className="review-note">{compare.kind === "working" ? "No changes since last commit." : "No file changes."}</StatusMessage>
          )}
          {files.length > 0 && <div className="review-files">{files.map((file) => {
            const fileStats = stats?.byPath.get(file.path)
            const blob = fileStats?.blob ?? ""
            return (
              <ReviewFile key={file.path} project={project} compare={compare} file={file} stats={fileStats}
                version={data?.version ?? ""}
                open={isOpen(file)} viewed={viewed(file.path, blob)} options={options} narrow={narrow} scrollRoot={scrollRoot}
                lines={selection?.path === file.path ? selection.lines : null}
                openFileReason={file.status === "deleted" ? "This file was deleted; there's no working copy to open"
                  : treeFiles.has(file.path) ? "" : "This file isn't in the working tree"}
                onOpenChange={(next) => setOpen((current) => new Map(current).set(file.path, next))}
                onViewedChange={(next) => {
                  onViewedChange(file.path, blob, next)
                  setOpen((current) => new Map(current).set(file.path, !next))
                }}
                onSelectLines={(lines) => {
                  pointer.current = `${file.path}#${lines ? diffLineAnchor(lines) : ""}`
                  onSelectLines(file.path, lines)
                }}
                onStatus={onStatus} onOpenFile={() => onOpenFile(file.path)} />
            )
          })}</div>}
        </div>
      </VirtualizerContext.Provider>
      </ShownContext.Provider>
    </ObserveContext.Provider>
  )
}

function reviewHeading(compare: Compare, data: CompareData | null, name: CompareName): string {
  if (data?.details.kind === "commit" && data.details.commit) return data.details.commit.subject || shortHash(data.details.commit.hash)
  if (compare.kind === "working") return "Uncommitted changes"
  return name.label
}

function ReviewMeta({ compare, data }: { compare: Compare; data: CompareData | null }): ReactNode {
  const details = data?.details
  if (details?.kind === "commit" && details.commit) {
    const { commit } = details
    return <>
      {commit.body && <p className="review-body">{commit.body}</p>}
      <p className="review-meta">
        <span title={commit.author.email}>{commit.author.name}</span>
        <span aria-hidden="true">·</span>
        <time dateTime={commit.authorTime}>{commitDate(commit.authorTime)}</time>
        <span aria-hidden="true">·</span>
        <code className="commit-compare-hash">{shortHash(commit.hash)}</code>
      </p>
    </>
  }
  if (details?.kind === "branch" && details.base) {
    return (
      <p className="review-meta">
        <span>{details.ahead.toLocaleString()} {details.ahead === 1 ? "commit" : "commits"} ahead of {details.base}</span>
        {details.mergeBase && <>
          <span aria-hidden="true">·</span>
          <span>merge-base</span>
          <code className="commit-compare-hash">{shortHash(details.mergeBase)}</code>
        </>}
      </p>
    )
  }
  if (compare.kind === "range") {
    const count = details?.kind === "range" ? details.count : 0
    return (
      <p className="review-meta">
        {count > 0 && <><span>{count.toLocaleString()} {count === 1 ? "commit" : "commits"}</span><span aria-hidden="true">·</span></>}
        <code className="commit-compare-hash">{shortHash(compare.from)}</code>
        <span>through</span>
        <code className="commit-compare-hash">{shortHash(compare.to)}</code>
      </p>
    )
  }
  return null
}

interface ReviewFileProps {
  project: string
  compare: Compare
  file: CompareFile
  stats: FileStats | undefined
  version: string
  open: boolean
  viewed: boolean
  options: DiffOptions
  narrow: boolean
  scrollRoot: RefObject<HTMLElement | null>
  lines: SelectedLineRange | null
  openFileReason: string
  onOpenChange(open: boolean): void
  onViewedChange(viewed: boolean): void
  onSelectLines(lines: SelectedLineRange | null): void
  onStatus(status: ReviewStatus | null): void
  onOpenFile(): void
}

// ReviewFile is one file's sticky header and collapsible diff.
function ReviewFile({
  compare, file, lines, narrow, open, openFileReason, options, project, scrollRoot, stats, version, viewed,
  onOpenChange, onOpenFile, onSelectLines, onStatus, onViewedChange,
}: ReviewFileProps) {
  const observe = useContext(ObserveContext)
  const section = useRef<HTMLElement>(null)
  const body = useRef<HTMLDivElement>(null)
  const [near, setNear] = useState(false)
  // height keeps a released diff's space, so scrolling stays put.
  const [height, setHeight] = useState(0)
  useLayoutEffect(() => {
    const element = section.current
    return element ? observe(element, setNear) : undefined
  }, [observe])
  const wanted = open && near
  const { diff, retry } = useReviewDiff({ project, compare, file, version, ignoreWhitespace: options.ignoreWhitespace, wanted })
  useLayoutEffect(() => {
    const element = body.current
    if (!element || !wanted) return
    const observer = new ResizeObserver(() => setHeight(element.offsetHeight))
    observer.observe(element)
    return () => observer.disconnect()
  }, [wanted])

  const fileDiff = diff.kind === "ready" ? diff.fileDiff : null
  const style = effectiveDiffStyle(options, narrow)
  const target = fileDiff && lines ? diffLineTarget(fileDiff, lines, style) : null
  const oldPath = diff.kind === "ready" && diff.diff.oldPath ? diff.diff.oldPath : file.oldPath ?? file.path
  const reference = target ? lineReference(target.side === "old" ? oldPath : file.path, target.range) : ""
  // The file holding the selection reports it to the status bar.
  const typeLabel = diff.kind === "ready" ? diff.diff.documentType.label : ""
  // Report by value: target is a new object every render, and reporting
  // rerenders this file.
  const selected = lines !== null
  const targetSide = target?.side ?? null
  const targetStart = target?.range.start ?? 0
  const targetEnd = target?.range.end ?? 0
  useEffect(() => {
    if (!selected) return
    const reported = targetSide ? { side: targetSide, range: { start: targetStart, end: targetEnd } } : null
    onStatus({ path: file.path, target: reported, typeLabel })
    return () => onStatus(null)
  }, [file.path, onStatus, selected, targetEnd, targetSide, targetStart, typeLabel])

  const label = statsLabel(stats)
  const bodyId = `review-${encodeURIComponent(file.path)}`
  return (
    <section ref={section} className="review-file" data-review-path={file.path} aria-label={file.path}>
      <div className="review-file-header">
        <Button variant="ghost" size="inline" className="review-file-toggle" aria-expanded={open} aria-controls={bodyId}
          onClick={() => onOpenChange(!open)}>
          <Chevron />
          <FileIcon path={file.path} />
          <span className="review-file-path" data-status={file.status}>{file.path}</span>
          {file.oldPath && <span className="review-file-from">from {file.oldPath}</span>}
        </Button>
        {label && <span className="review-file-stats">{label}</span>}
        <span className="commit-file-status" data-status={file.status} title={statusNames[file.status]}>{statusLetters[file.status]}</span>
        <ToggleButton size="text" className="review-viewed" pressed={viewed} disabled={!stats?.blob}
          title={stats?.blob ? "Mark as viewed; changes to the file unmark it" : "Waiting for the file's content hash"}
          onPressedChange={onViewedChange}>
          {viewed && <CheckIcon aria-hidden="true" />}Viewed
        </ToggleButton>
        <IconButton size="xs" label="Open File" icon={<FileInputIcon aria-hidden="true" />}
          disabledReason={openFileReason || undefined} onClick={onOpenFile} />
      </div>
      <div id={bodyId} ref={body} className="review-file-body" hidden={!open}
        style={open && !wanted && height > 0 ? { minHeight: height } : undefined}>
        {wanted && <FileBody diff={diff} file={file} compare={compare} fileDiff={fileDiff} options={options} narrow={narrow}
          scrollRoot={scrollRoot} lines={lines} reference={reference} onRetry={retry} onSelectLines={onSelectLines} />}
      </div>
    </section>
  )
}

interface FileBodyProps {
  diff: ReturnType<typeof useReviewDiff>["diff"]
  file: CompareFile
  compare: Compare
  fileDiff: FileDiffMetadata | null
  options: DiffOptions
  narrow: boolean
  scrollRoot: RefObject<HTMLElement | null>
  lines: SelectedLineRange | null
  reference: string
  onRetry(): void
  onSelectLines(lines: SelectedLineRange | null): void
}

function FileBody({ compare, diff, file, fileDiff, lines, narrow, options, reference, scrollRoot, onRetry, onSelectLines }: FileBodyProps) {
  if (diff.kind === "loading") return <StatusMessage className="review-file-note">Loading diff…</StatusMessage>
  if (diff.kind === "error") {
    return (
      <div className="review-file-note" role="alert">
        <StatusMessage tone="error">{diff.message}</StatusMessage>
        <Button variant="outline" onClick={onRetry}>Retry</Button>
      </div>
    )
  }
  if (diff.kind === "unchanged") {
    return <StatusMessage className="review-file-note">
      No changes to <code>{file.path}</code> {compare.kind === "working" ? "since the last commit" : "in this compare"}.
    </StatusMessage>
  }
  const { content, parseFailed } = diff
  return <>
    {content.kind === "binary" && <Notice tone="warning" layout="bar" role="note">{binaryChangeLabel(content.before, content.after)}</Notice>}
    {content.kind === "tooLarge" && <Notice tone="warning" layout="bar" role="note">Large file ({formatBytes(content.size)}): diff is off.</Notice>}
    {content.kind === "text" && parseFailed && <Notice tone="error" layout="bar">Couldn't compute this diff.</Notice>}
    {fileDiff && fileDiff.hunks.length === 0 && (
      <Notice layout="bar" role="note">{options.ignoreWhitespace ? "No changes apart from whitespace." : "File contents are unchanged."}</Notice>
    )}
    {fileDiff && fileDiff.hunks.length > 0 && <DiffView path={file.path} fileDiff={fileDiff} narrow={narrow} options={options} scrollRoot={scrollRoot}
      lines={lines} reference={reference} onSelectLines={onSelectLines} />}
  </>
}

interface DiffViewProps {
  path: string
  fileDiff: FileDiffMetadata
  narrow: boolean
  options: DiffOptions
  scrollRoot: RefObject<HTMLElement | null>
  lines: SelectedLineRange | null
  reference: string
  onSelectLines(lines: SelectedLineRange | null): void
}

type DiffInstance = VirtualizedFileDiff<undefined, undefined>

// DiffView renders only the rows near the pane's viewport.
function DiffView({ fileDiff, lines, narrow, options, path, reference, scrollRoot, onSelectLines }: DiffViewProps) {
  const { resolvedTheme } = useTheme()
  // Pierre hands over its virtualized instance, which places and expands
  // rows; the review reveals and steps through it.
  const register = useContext(ShownContext)
  const host = useRef<HTMLDivElement>(null)
  const [instance, setInstance] = useState<DiffInstance | null>(null)
  const onInstance = useCallback((next: FileDiffInstance<undefined, undefined> | null) => {
    setInstance(next instanceof VirtualizedFileDiff ? next : null)
  }, [])
  useEffect(() => {
    if (!instance) return
    register(path, {
      fileDiff,
      // The diff's place comes from the DOM: Pierre's own top trails a
      // scroll until its next frame.
      lineBox(side, line) {
        const position = instance.getLinePosition(line, side)
        const root = scrollRoot.current
        const element = host.current
        if (!position || !root || !element) return null
        const top = element.getBoundingClientRect().top - root.getBoundingClientRect().top + root.scrollTop
        return { top: top + position.top, height: position.height }
      },
      expandTo(side, line) {
        const gap = gapForLine(fileDiff, side, line)
        if (gap && !instance.isLineRenderable(gap.newLine)) instance.expandHunk(gap.hunkIndex, "up", gap.lines)
      },
    })
    return () => register(path, null)
  }, [fileDiff, instance, path, register, scrollRoot])
  // Escape clears the selection while focus is in the pane, as in Source.
  const hasSelection = lines !== null
  useEffect(() => {
    const root = scrollRoot.current
    if (!root || !hasSelection) return
    const clear = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented) return
      event.preventDefault()
      onSelectLines(null)
    }
    root.addEventListener("keydown", clear)
    return () => root.removeEventListener("keydown", clear)
  }, [hasSelection, onSelectLines, scrollRoot])
  // Options stay stable across renders: new ones would close the gaps a
  // reveal opened.
  const select = useRef(onSelectLines)
  useLayoutEffect(() => { select.current = onSelectLines })
  // Pierre reads row metrics once, so a Font Size change remounts the diff.
  const scale = uiScale(useSettings().settings)
  const metrics = useMemo(() => diffMetrics(scale), [scale])
  const inline = lineDiffType(options.inline)
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
    // Only pointer gestures end a selection. onLineSelected also echoes
    // selectedLines changes, which would clear a selection moving to
    // another file.
    onLineSelectionEnd(range) {
      // Selection moves focus to the pane so Escape clears it.
      scrollRoot.current?.focus({ preventScroll: true })
      select.current(range)
    },
  }), [inline, narrow, options, resolvedTheme, scrollRoot])
  return (
    <div ref={host}>
      <FileDiff key={metrics.lineHeight} className="diff-view" fileDiff={fileDiff} options={diffOptions} metrics={metrics}
        selectedLines={lines} onInstance={onInstance}
        renderGutterUtility={() => lines && reference
          ? <span className="diff-line-copy"><LineCopyButton reference={reference} /></span>
          : null} />
    </div>
  )
}
