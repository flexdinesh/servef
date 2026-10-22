import { formatBytes } from "./format-bytes.ts"
import type { PageData } from "./page-data.ts"
import { projectUrl } from "./project-url.ts"
import { useProcessMetrics } from "./process-metrics.ts"
import { CheckIcon, CopyIcon, SearchIcon, SettingsIcon } from "lucide-react"
import { useClipboardCopy } from "./clipboard.ts"
import { lineRangeLabel, type LineRange } from "./line-selection.ts"
import { settingsShortcutLabel } from "./SettingsDialog.tsx"
import { NavigationStatusPanel, type NavigationStatusPanelProps } from "./NavigationStatusPanel.tsx"
import { diffStatsLabel, type DiffStats } from "./features/source-control/diff-stats.ts"
import type { DiffLineTarget } from "./features/source-control/diff-line-selection.ts"

// DiffStatus describes the active diff tab, which replaces document status.
export interface DiffStatus {
  lines: DiffLineTarget | null
  path: string
  stats: DiffStats | null
  typeLabel: string
}

interface StatusBarProps {
  detail?: string
  diff?: DiffStatus
  isLoading: boolean
  lines?: LineRange | null
  // navigation shows who answers code navigation for the open file.
  navigation?: NavigationStatusPanelProps
  page: PageData
  error: string
  onSearch(): void
  onSettings(): void
  onHome?(): void
}

export function StatusBar({ detail, diff, isLoading, error, lines, navigation, onSearch, onSettings, onHome, page }: StatusBarProps) {
  const metrics = useProcessMetrics()
  const pathCopy = useClipboardCopy()
  const processTitle = metrics
    ? [
        "servef process",
        metrics.memorySource === "rss"
          ? `Resident RAM: ${formatBytes(metrics.memoryBytes)}`
          : `Go runtime system memory: ${formatBytes(metrics.memoryBytes)}`,
        ...(metrics.cpuUsage === null ? [] : [`CPU: ${metrics.cpuUsage.toFixed(1)}%`]),
        `Goroutines: ${metrics.goroutines}`,
      ].join("\n")
    : undefined

  return (
    <footer className="status-bar">
      <div className="status-explorer" role="group" aria-label="File tree status">
        <a className="status-brand" href={page.project ? projectUrl("/", page.project) : "/"} aria-label="servef home" onClick={(event) => {
          if (!onHome || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return
          event.preventDefault()
          onHome()
        }}>servef</a>
        <details className="status-process" onKeyDown={(event) => {
          if (event.key === "Escape") event.currentTarget.open = false
        }}>
          <summary className={`status-state${isLoading ? " loading" : error ? " failed" : ""}`} title="Process information">
            {isLoading ? "Loading…" : error ? "Error" : "Ready"}
          </summary>
          <div className="status-process-panel">
            <strong>servef process</strong>
            {error && <p>{error}</p>}
            {metrics ? <p>{processTitle?.split("\n").slice(1).map((line) => <span key={line}>{line}</span>)}</p>
              : <p>Process metrics unavailable.</p>}
          </div>
        </details>
        <span className="status-grow" />
        <span className="status-file-count">{page.fileCount.toLocaleString()} files</span>
      </div>
      <div className="status-document" role="group" aria-label="Document status">
        <span className="status-path" title={diff?.path || page.selected || page.rootName}>
          {diff?.path || page.selected || page.rootName || "servef"}
        </span>
        {(diff || (page.hasFile && page.selected)) && (
          <button
            className="status-copy-path"
            type="button"
            aria-label="Copy path"
            title={pathCopy.status === "failed" ? "Copy failed" : "Copy path"}
            data-status={pathCopy.status}
            onClick={() => { void pathCopy.copy(diff?.path ?? page.selected) }}
          >
            {pathCopy.status === "copied" ? <CheckIcon aria-hidden="true" /> : <CopyIcon aria-hidden="true" />}
          </button>
        )}
        <button className="status-search" type="button" aria-label="Find in files" title="Find in files" aria-keyshortcuts="Control+Shift+F Meta+Shift+F" onClick={onSearch}>
          <SearchIcon aria-hidden="true" />
          <span>Find in files</span>
          <kbd>{navigator.platform.includes("Mac") ? "⌘⇧F" : "Ctrl+Shift+F"}</kbd>
        </button>
        <span className="status-grow" />
        {diff ? <>
          {diff.lines && <span className="status-lines">
            {diff.lines.side === "old" ? `Old ${lineRangeLabel(diff.lines.range)}` : lineRangeLabel(diff.lines.range)}
          </span>}
          {diff.stats && <span className="status-diff-stats" title={`${diff.stats.additions.toLocaleString()} added, ${diff.stats.deletions.toLocaleString()} deleted lines`}>
            {diffStatsLabel(diff.stats)}
          </span>}
          {diff.typeLabel && <span className="status-file-type">{diff.typeLabel}</span>}
        </> : <>
          {page.hasFile && lines && <span className="status-lines">{lineRangeLabel(lines)}</span>}
          {page.hasFile && detail && <span className="status-document-detail">{detail}</span>}
          {page.hasFile && <span className="status-file-size">{formatBytes(page.fileSize)}</span>}
          {page.hasFile && navigation && <NavigationStatusPanel {...navigation} />}
          {page.hasFile && page.documentType && (
            <span className="status-file-type">
              {page.binary && !page.documentType.previewKind ? "Binary" : page.documentType.label}
            </span>
          )}
        </>}
        <button
          className="status-settings"
          type="button"
          aria-haspopup="dialog"
          aria-keyshortcuts="Control+, Meta+,"
          aria-label="Settings"
          title={`Settings (${settingsShortcutLabel})`}
          onClick={onSettings}
        >
          <SettingsIcon aria-hidden="true" />
        </button>
        <span className="visually-hidden" role="status">
          {pathCopy.status === "copied" ? "Path copied" : pathCopy.status === "failed" ? "Copy failed" : ""}
        </span>
      </div>
    </footer>
  )
}
