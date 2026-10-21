import type { FileContents, FileDiffMetadata, Hunk } from "@pierre/diffs"

// A diff parse runs jsdiff over both sides in a worker, as megabyte sides
// block the main thread for seconds.

export type DiffWorkerRequest = {
  type: "parse"
  requestId: number
  oldFile: FileContents | null
  newFile: FileContents | null
  ignoreWhitespace: boolean
}

export interface DiffWorker extends EventTarget {
  postMessage(message: DiffWorkerRequest): void
  terminate(): void
}

interface PendingParse {
  request: DiffWorkerRequest
  resolve(fileDiff: FileDiffMetadata): void
  reject(error: Error): void
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null
}

const changeTypes: ReadonlySet<unknown> = new Set(["change", "rename-pure", "rename-changed", "new", "deleted"])

function isHunk(value: unknown): value is Hunk {
  return isRecord(value)
    && typeof value.additionStart === "number" && typeof value.additionCount === "number"
    && typeof value.additionLines === "number" && typeof value.additionLineIndex === "number"
    && typeof value.deletionStart === "number" && typeof value.deletionCount === "number"
    && typeof value.deletionLines === "number" && typeof value.deletionLineIndex === "number"
    && typeof value.splitLineStart === "number" && typeof value.unifiedLineStart === "number"
    && Array.isArray(value.hunkContent) && value.hunkContent.every(isRecord)
}

function isLines(value: unknown): value is string[] {
  return Array.isArray(value) && value.every((line) => typeof line === "string")
}

// isFileDiffMetadata checks the worker's reply before Pierre renders it.
function isFileDiffMetadata(value: unknown): value is FileDiffMetadata {
  return isRecord(value)
    && typeof value.name === "string"
    && changeTypes.has(value.type)
    && typeof value.splitLineCount === "number"
    && typeof value.unifiedLineCount === "number"
    && typeof value.isPartial === "boolean"
    && Array.isArray(value.hunks) && value.hunks.every(isHunk)
    && isLines(value.deletionLines) && isLines(value.additionLines)
}

function abortError(): Error {
  return new DOMException("The diff parse was aborted.", "AbortError")
}

// createDiffParser shares one lazily created worker. Replies match requests by
// id; aborting a request that is still pending discards its reply and replaces
// the busy worker, so stale work never delays the next diff.
export function createDiffParser(createWorker: () => DiffWorker) {
  let worker: DiffWorker | undefined
  let nextRequestId = 0
  const pending = new Map<number, PendingParse>()

  const onMessage = (event: Event): void => {
    if (event.target !== worker || !(event instanceof MessageEvent) || !isRecord(event.data)) return
    const { requestId, type, fileDiff } = event.data
    const parse = typeof requestId === "number" ? pending.get(requestId) : undefined
    if (!parse || typeof requestId !== "number") return
    pending.delete(requestId)
    if (type === "parsed" && isFileDiffMetadata(fileDiff)) parse.resolve(fileDiff)
    else parse.reject(new Error("Couldn't compute this diff."))
  }

  const onError = (event: Event): void => {
    if (event.target !== worker) return
    dispose()
    const failed = [...pending.values()]
    pending.clear()
    for (const parse of failed) parse.reject(new Error("The diff worker failed."))
  }

  function dispose(): void {
    worker?.removeEventListener("message", onMessage)
    worker?.removeEventListener("error", onError)
    worker?.terminate()
    worker = undefined
  }

  function post(request: DiffWorkerRequest): void {
    if (!worker) {
      worker = createWorker()
      worker.addEventListener("message", onMessage)
      worker.addEventListener("error", onError)
    }
    worker.postMessage(request)
  }

  const parse = (oldFile: FileContents | null, newFile: FileContents | null, ignoreWhitespace: boolean, signal: AbortSignal): Promise<FileDiffMetadata> => {
    if (signal.aborted) return Promise.reject(abortError())
    const request: DiffWorkerRequest = { type: "parse", requestId: ++nextRequestId, oldFile, newFile, ignoreWhitespace }
    return new Promise<FileDiffMetadata>((resolve, reject) => {
      const onAbort = (): void => {
        if (!pending.delete(request.requestId)) return
        reject(abortError())
        // The worker may still be busy with this parse; others restart on a fresh one.
        dispose()
        for (const queued of pending.values()) post(queued.request)
      }
      pending.set(request.requestId, {
        request,
        resolve(fileDiff) {
          signal.removeEventListener("abort", onAbort)
          resolve(fileDiff)
        },
        reject(error) {
          signal.removeEventListener("abort", onAbort)
          reject(error)
        },
      })
      signal.addEventListener("abort", onAbort, { once: true })
      try {
        post(request)
      } catch {
        pending.delete(request.requestId)
        signal.removeEventListener("abort", onAbort)
        reject(new Error("The diff worker is unavailable."))
      }
    })
  }

  // release ends the worker and its pending parses once no diff needs them;
  // the next parse starts a fresh worker.
  const release = (): void => {
    const released = [...pending.values()]
    pending.clear()
    dispose()
    for (const parse of released) parse.reject(abortError())
  }

  return { parse, release }
}
