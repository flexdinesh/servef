import { isProcessMetrics, type ProcessMetrics } from "@srcz/contracts"
import { useEffect, useState } from "react"

import { fetchJSON } from "../api/http.ts"

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
