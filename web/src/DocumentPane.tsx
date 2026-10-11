import type { RefObject } from "react"

import { Button } from "@/components/ui/button"

import { DocumentPreview } from "./DocumentPreview.tsx"
import { FormattedDocument } from "./FormattedDocument.tsx"
import { SourceDocument } from "./SourceDocument.tsx"
import type { DocumentMode } from "./DocumentModeToggle.tsx"
import type { PageData } from "./page-data.ts"

interface DocumentPaneProps {
  hasData: boolean
  isLoading: boolean
  loadError: string
  main: RefObject<HTMLElement | null>
  mode: DocumentMode
  navigate(href: string): void
  page: PageData
  retry(): void
}

export function DocumentPane({ hasData, isLoading, loadError, main, mode, navigate, page, retry }: DocumentPaneProps) {
  const sourceOnly = !page.documentType?.previewKind
  const showSource = !loadError && !page.error && page.hasFile && (mode === "source" || sourceOnly)
  const showFormatted = !loadError && !page.error && !page.previewError && page.hasFile && mode === "preview" && page.documentType?.previewKind === "code"

  return (
    <main
      className={showSource || showFormatted ? "source-mode" : undefined}
      id="document-pane"
      ref={main}
      tabIndex={-1}
      aria-busy={isLoading}
    >
      {loadError ? (
        <div className="error" role="alert">
          <p>{loadError}</p>
          <Button type="button" variant="outline" onClick={retry}>Try again</Button>
        </div>
      ) : page.error ? (
        <div className="error" role="alert">{page.error}</div>
      ) : page.hasFile ? (
        (mode === "source" || sourceOnly) ? (
          <SourceDocument key={page.selected} source={page.source} highlightedLines={page.sourceLines} label={`${page.documentType?.label ?? "File"} source`} />
        ) : (
          page.previewError ? <div className="error" role="alert">
            <p>Could not format this file: {page.previewError}</p>
            <p>Switch to Source to read the original file.</p>
          </div> : page.documentType?.previewKind === "code" ? (
            <FormattedDocument key={page.selected} html={page.content} label={`${page.documentType.label} formatted`} />
          ) : <DocumentPreview key={page.selected} html={page.content} navigate={navigate} />
        )
      ) : hasData ? (
        <div className="empty">
          <h1>Choose a file</h1>
          <p>Select a file from the folder tree.</p>
        </div>
      ) : null}
    </main>
  )
}
