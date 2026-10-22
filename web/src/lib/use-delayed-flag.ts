import { useEffect, useState } from "react"

// useDelayedFlag reports an active state only after it has lasted `delay` ms.
export function useDelayedFlag(active: boolean, delay: number): boolean {
  const [elapsed, setElapsed] = useState(false)
  useEffect(() => {
    if (!active) return
    const timer = window.setTimeout(() => setElapsed(true), delay)
    return () => {
      window.clearTimeout(timer)
      setElapsed(false)
    }
  }, [active, delay])
  return active && elapsed
}
