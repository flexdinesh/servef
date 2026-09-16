import { useEffect, useState } from "react"

export interface ProcessMetrics {
  cpuUsage: number | null
  goroutines: number
  memoryBytes: number
  memorySource: "go" | "rss"
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

export function isProcessMetrics(value: unknown): value is ProcessMetrics {
  return isRecord(value)
    && (value.cpuUsage === null || typeof value.cpuUsage === "number")
    && typeof value.goroutines === "number"
    && typeof value.memoryBytes === "number"
    && (value.memorySource === "go" || value.memorySource === "rss")
}

export function useProcessMetrics(): ProcessMetrics | null {
  const [metrics, setMetrics] = useState<ProcessMetrics | null>(null)

  useEffect(() => {
    let disposed = false

    const refresh = async () => {
      if (document.hidden) return
      try {
        const response = await fetch("/api/metrics")
        const payload: unknown = await response.json()
        if (!disposed && response.ok && isProcessMetrics(payload)) setMetrics(payload)
      } catch {
        // Keep the last sample when the local server is briefly unavailable.
      }
    }

    const refreshWhenVisible = () => { void refresh() }
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 2_500)
    document.addEventListener("visibilitychange", refreshWhenVisible)
    return () => {
      disposed = true
      window.clearInterval(timer)
      document.removeEventListener("visibilitychange", refreshWhenVisible)
    }
  }, [])

  return metrics
}
