import { isRecord } from "../../api/http.ts"
import { createSearchEngine, type SearchDocument, type SearchQuery } from "./search-engine.ts"

function isSearchDocument(value: unknown): value is SearchDocument {
  return isRecord(value)
    && typeof value.path === "string"
    && typeof value.name === "string"
    && typeof value.content === "string"
}

function isSearchQuery(value: unknown): value is SearchQuery {
  return isRecord(value)
    && typeof value.pattern === "string"
    && typeof value.include === "string"
    && typeof value.exclude === "string"
    && typeof value.matchCase === "boolean"
    && typeof value.wholeWord === "boolean"
    && typeof value.regex === "boolean"
}

const engine = createSearchEngine()

self.addEventListener("message", (event: MessageEvent<unknown>) => {
  const message = event.data
  if (!isRecord(message)) return

  try {
    if (message.type === "init") {
      const documents = Array.isArray(message.documents) ? message.documents.filter(isSearchDocument) : []
      engine.initialize(documents)
      self.postMessage({ type: "ready" })
    } else if (message.type === "search" && isSearchQuery(message.query)) {
      self.postMessage({ type: "results", requestId: message.requestId, outcome: engine.search(message.query) })
    }
  } catch {
    self.postMessage({ type: "error", requestId: message.requestId })
  }
})
