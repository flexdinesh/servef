import { useEffect, useRef, type RefObject } from "react"

// Focus and visibility refreshes run at most this often.
const focusThrottle = 2_000

interface RefreshOnReturnOptions {
  // lastRead is when the data was last read (Date.now()); a return sooner
  // than the throttle after it does not refresh.
  lastRead: RefObject<number>
  // enabled false stops listening, e.g. for data that never changes.
  enabled?: boolean
  // viewActive rereads when the view opens; staying open does not.
  viewActive?: boolean
}

// useRefreshOnReturn rereads when the user comes back: on window focus or
// visibility (throttled), and when the view showing the data opens. It never
// polls.
export function useRefreshOnReturn(refresh: () => void, { lastRead, enabled = true, viewActive }: RefreshOnReturnOptions) {
  const wasActive = useRef(viewActive)
  useEffect(() => {
    if (viewActive && !wasActive.current) refresh()
    wasActive.current = viewActive
  }, [refresh, viewActive])

  useEffect(() => {
    if (!enabled) return
    const onReturn = () => {
      if (document.visibilityState !== "visible" || Date.now() - lastRead.current < focusThrottle) return
      refresh()
    }
    window.addEventListener("focus", onReturn)
    document.addEventListener("visibilitychange", onReturn)
    return () => {
      window.removeEventListener("focus", onReturn)
      document.removeEventListener("visibilitychange", onReturn)
    }
  }, [enabled, lastRead, refresh])
}
