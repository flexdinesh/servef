import type { PageData } from "./page-data.ts"
import { useProcessMetrics } from "./process-metrics.ts"
import { SearchIcon } from "lucide-react"
import { ModeToggle } from "./components/ModeToggle.tsx"

interface StatusBarProps {
  isLoading: boolean
  page: PageData
  error: string
  onSearch(): void
  onHome?(): void
}

function formatBytes(bytes: number): string {
  if (bytes < 1_024) return `${bytes} B`
  if (bytes < 1_048_576) return `${(bytes / 1_024).toFixed(1)} KB`
  if (bytes < 1_073_741_824) return `${(bytes / 1_048_576).toFixed(1)} MB`
  return `${(bytes / 1_073_741_824).toFixed(1)} GB`
}

export function StatusBar({ isLoading, error, onSearch, onHome, page }: StatusBarProps) {
  const metrics = useProcessMetrics()
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
        <a className="status-brand" href="/" aria-label="servef home" onClick={(event) => {
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
        <span className="status-path" title={page.selected || page.rootName}>
          {page.selected || page.rootName || "servef"}
        </span>
        <button className="status-search" type="button" aria-keyshortcuts="Control+Shift+F Meta+Shift+F" onClick={onSearch}>
          <SearchIcon aria-hidden="true" />
          <span>Find in files</span>
          <kbd>{navigator.platform.includes("Mac") ? "⌘⇧F" : "Ctrl+Shift+F"}</kbd>
        </button>
        <span className="status-grow" />
        {page.hasFile && <span className="status-file-size">{formatBytes(page.fileSize)}</span>}
        <ModeToggle />
      </div>
    </footer>
  )
}
