import { useEffect, useState } from "react"

import { fetchJSON, isRecord } from "../http.ts"

export interface ProcessMetrics {
  cpuUsage: number | null
  goroutines: number
  memoryBytes: number
  memorySource: "go" | "rss"
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
    let pending: AbortController | null = null

    const refresh = async () => {
      if (disposed || document.hidden || pending) return
      const controller = new AbortController()
      pending = controller
      try {
        const payload = await fetchJSON("/api/metrics", { signal: controller.signal }, isProcessMetrics, () => "Couldn't read process metrics.")
        if (!disposed) setMetrics(payload)
      } catch {
        // Keep the last sample when the local server is briefly unavailable.
      } finally {
        pending = null
      }
    }

    const refreshWhenVisible = () => { void refresh() }
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, 2_500)
    document.addEventListener("visibilitychange", refreshWhenVisible)
    return () => {
      disposed = true
      pending?.abort()
      window.clearInterval(timer)
      document.removeEventListener("visibilitychange", refreshWhenVisible)
    }
  }, [])

  return metrics
}
