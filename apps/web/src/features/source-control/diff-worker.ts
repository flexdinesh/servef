import type { FileContents } from "@pierre/diffs"

import { parseTextDiff } from "./diff-content.ts"
import { isRecord } from "../../api/http.ts"

function isSide(value: unknown): value is FileContents | null {
  return value === null || (isRecord(value)
    && typeof value.name === "string"
    && typeof value.contents === "string"
    && (value.lang === undefined || typeof value.lang === "string"))
}

self.addEventListener("message", (event: MessageEvent<unknown>) => {
  const message = event.data
  if (!isRecord(message) || message.type !== "parse") return
  const { requestId, oldFile, newFile, ignoreWhitespace } = message
  try {
    if (!isSide(oldFile) || !isSide(newFile) || typeof ignoreWhitespace !== "boolean") throw new TypeError("Invalid diff request")
    self.postMessage({ type: "parsed", requestId, fileDiff: parseTextDiff({ kind: "text", oldFile, newFile }, ignoreWhitespace) })
  } catch {
    self.postMessage({ type: "error", requestId })
  }
})
