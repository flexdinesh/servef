import type { PageData } from "./page-data.ts"
import type { ProcessMetrics } from "./process-metrics.ts"

interface StatusBarProps {
  isLoading: boolean
  metrics: ProcessMetrics | null
  page: PageData
}

function formatBytes(bytes: number): string {
  if (bytes < 1_024) return `${bytes} B`
  if (bytes < 1_048_576) return `${(bytes / 1_024).toFixed(1)} KB`
  if (bytes < 1_073_741_824) return `${(bytes / 1_048_576).toFixed(1)} MB`
  return `${(bytes / 1_073_741_824).toFixed(1)} GB`
}

export function StatusBar({ isLoading, metrics, page }: StatusBarProps) {
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
      <span className={`status-state${isLoading ? " loading" : ""}`}>
        {isLoading ? "Loading…" : "Ready"}
      </span>
      <span className="status-path" title={page.selected || page.rootName}>
        {page.selected || page.rootName || "servef"}
      </span>
      <span className="status-grow" />
      {page.hasFile && <span className="status-file-size">{formatBytes(page.fileSize)}</span>}
      <span>{page.fileCount.toLocaleString()} files</span>
      {metrics && (
        <span className="status-metrics" title={processTitle}>
          {metrics.cpuUsage !== null && (
            <>
              <span><span className="status-metric-label">CPU </span>{metrics.cpuUsage.toFixed(1)}%</span>
              <span aria-hidden="true">·</span>
            </>
          )}
          <span>
            <span className="status-metric-label">{metrics.memorySource === "rss" ? "RAM " : "MEM "}</span>
            {formatBytes(metrics.memoryBytes)}
          </span>
        </span>
      )}
    </footer>
  )
}
