import { loadHighlights } from "./highlights-loader.ts"
import { isRecord } from "../http.ts"
import { highlightLines } from "./syntax-highlight.ts"

// The highlight worker tokenizes Source and Format text above the main-thread
// limit (highlight-thread.ts), so large files never block the UI.

self.addEventListener("message", (event: MessageEvent<unknown>) => {
  const message = event.data
  if (!isRecord(message) || message.type !== "highlight") return
  const { requestId, source, language } = message
  if (typeof requestId !== "number") return
  if (typeof source !== "string" || typeof language !== "string") {
    self.postMessage({ type: "highlighted", requestId, lines: null })
    return
  }
  void loadHighlights().then((highlights) => {
    self.postMessage({ type: "highlighted", requestId, lines: highlights ? highlightLines(highlights, source, language) : null })
  })
})
