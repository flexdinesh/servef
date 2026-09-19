import type { RefObject } from "react"

import { Button } from "@/components/ui/button"

import { MarkdownDocument } from "./MarkdownDocument.tsx"
import { MarkdownSource } from "./MarkdownSource.tsx"
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
  const showSource = !loadError && !page.error && page.hasFile && mode === "source"

  return (
    <main
      className={showSource ? "source-mode" : undefined}
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
        mode === "source" ? (
          <MarkdownSource source={page.source} />
        ) : (
          <MarkdownDocument key={page.selected} html={page.content} navigate={navigate} />
        )
      ) : hasData ? (
        <div className="empty">
          <div className="empty-mark" aria-hidden="true">M</div>
          <h1>Choose a Markdown file</h1>
          <p>Select a Markdown file from the folder tree.</p>
        </div>
      ) : null}
    </main>
  )
}
