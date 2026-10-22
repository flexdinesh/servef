import { highlightsOnMainThread } from "./highlight-thread.ts"
import { loadHighlights } from "./highlights-loader.ts"
import { isRecord } from "../http.ts"
import { highlightLines } from "./syntax-highlight.ts"

export interface HighlightWorker extends EventTarget {
  postMessage(message: { type: "highlight"; requestId: number; source: string; language: string }): void
  terminate(): void
}

function isLines(value: unknown): value is string[] | null {
  return value === null || (Array.isArray(value) && value.every((line) => typeof line === "string"))
}

// createSourceHighlighter highlights Source and Format text: small text on
// the main thread, larger text in one lazily created worker (decision 0029).
// Results are escaped HTML per viewer row, or null for plain text. Aborted
// requests discard their reply.
export function createSourceHighlighter(createWorker: () => HighlightWorker) {
  let worker: HighlightWorker | undefined
  let nextRequestId = 0
  const pending = new Map<number, (lines: string[] | null) => void>()

  const onMessage = (event: Event): void => {
    if (event.target !== worker || !(event instanceof MessageEvent) || !isRecord(event.data)) return
    const { requestId, lines } = event.data
    const resolve = typeof requestId === "number" ? pending.get(requestId) : undefined
    if (!resolve || typeof requestId !== "number") return
    pending.delete(requestId)
    resolve(isLines(lines) ? lines : null)
  }

  // A failed worker leaves its text plain; the next request starts a new one.
  const onError = (event: Event): void => {
    if (event.target !== worker) return
    release()
  }

  function release(): void {
    worker?.removeEventListener("message", onMessage)
    worker?.removeEventListener("error", onError)
    worker?.terminate()
    worker = undefined
    const released = [...pending.values()]
    pending.clear()
    for (const resolve of released) resolve(null)
  }

  function inWorker(source: string, language: string, signal: AbortSignal): Promise<string[] | null> {
    const requestId = ++nextRequestId
    return new Promise((resolve) => {
      const onAbort = (): void => {
        if (pending.delete(requestId)) resolve(null)
      }
      pending.set(requestId, (lines) => {
        signal.removeEventListener("abort", onAbort)
        resolve(lines)
      })
      signal.addEventListener("abort", onAbort, { once: true })
      try {
        if (!worker) {
          worker = createWorker()
          worker.addEventListener("message", onMessage)
          worker.addEventListener("error", onError)
        }
        worker.postMessage({ type: "highlight", requestId, source, language })
      } catch {
        pending.delete(requestId)
        signal.removeEventListener("abort", onAbort)
        resolve(null)
      }
    })
  }

  async function highlight(source: string, language: string, signal: AbortSignal): Promise<string[] | null> {
    if (!highlightsOnMainThread(source.length)) return inWorker(source, language, signal)
    const highlights = await loadHighlights()
    return highlights && !signal.aborted ? highlightLines(highlights, source, language) : null
  }

  return { highlight, release }
}
