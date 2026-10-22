import { isRecord } from "./http.ts"
import type { SearchDocument } from "./search-engine.ts"

export interface SearchDocumentsResponse {
  documents: SearchDocument[]
  warnings: string[]
}

function isSearchDocument(value: unknown): value is SearchDocument {
  return isRecord(value)
    && typeof value.content === "string"
    && typeof value.name === "string"
    && typeof value.path === "string"
}

export function isSearchDocumentsResponse(value: unknown): value is SearchDocumentsResponse {
  return isRecord(value)
    && Array.isArray(value.documents)
    && value.documents.every(isSearchDocument)
    && Array.isArray(value.warnings)
    && value.warnings.every((warning) => typeof warning === "string")
}
