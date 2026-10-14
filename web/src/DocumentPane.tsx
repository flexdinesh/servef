import type { RefObject } from "react"

import { Button } from "@/components/ui/button"

import { BinaryDocument, TooLargeDocument } from "./BinaryDocument.tsx"
import { FormatErrorNotice, LargeFileNotice } from "./DocumentNotice.tsx"
import { DocumentPreview } from "./DocumentPreview.tsx"
import type { DocumentView } from "./document-view.ts"
import { FormattedDocument } from "./FormattedDocument.tsx"
import { handleImageKey, ImageDocument } from "./ImageDocument.tsx"
import { SourceDocument, type LineSelection } from "./SourceDocument.tsx"
import type { PageData } from "./page-data.ts"
import { projectUrl } from "./project-url.ts"
import type { ImageViewer } from "./use-image-viewer.ts"

interface DocumentPaneProps {
  hasData: boolean
  image: ImageViewer
  isLoading: boolean
  lineSelection: LineSelection
  loadError: string
  main: RefObject<HTMLElement | null>
  navigate(href: string): void
  onChooseProject(): void
  page: PageData
  retry(): void
  view: DocumentView
}

function fileName(path: string): string {
  return path.split("/").at(-1) || path
}

export function DocumentPane({ hasData, image, isLoading, lineSelection, loadError, main, navigate, onChooseProject, page, retry, view }: DocumentPaneProps) {
  const viewKind = loadError || page.error ? "none" : view.kind
  const label = page.documentType?.label ?? "File"

  return (
    <main
      className={viewKind === "source" || viewKind === "format" ? "source-mode" : viewKind === "image" ? "image-mode" : undefined}
      id="document-pane"
      ref={main}
      tabIndex={-1}
      aria-busy={isLoading}
      onKeyDown={viewKind === "image" ? (event) => handleImageKey(event, image) : undefined}
    >
      {loadError ? (
        <div className="error" role="alert">
          <p>{loadError}</p>
          <Button type="button" variant="outline" onClick={retry}>Try again</Button>
        </div>
      ) : page.projectUnavailable ? (
        <div className="error project-unavailable" role="alert">
          <p>The project <code>{page.project}</code> is unavailable. It may have been moved or deleted.</p>
          <Button type="button" variant="outline" onClick={onChooseProject}>Choose another project</Button>
        </div>
      ) : page.error ? (
        <div className="error" role="alert">{page.error}</div>
      ) : view.kind === "binary" ? (
        <BinaryDocument name={fileName(page.selected)} size={page.fileSize} />
      ) : view.kind === "tooLarge" ? (
        <TooLargeDocument name={fileName(page.selected)} size={page.fileSize} />
      ) : view.kind === "image" ? (
        <ImageDocument
          key={page.selected}
          name={fileName(page.selected)}
          src={projectUrl("/api/raw", page.project, page.selected)}
          viewer={image}
        />
      ) : view.kind === "preview" ? (
        page.previewError ? (
          <div className="error" role="alert">
            <p>Could not render this preview: {page.previewError}</p>
            <p>Switch to Source to read the original file.</p>
          </div>
        ) : <DocumentPreview key={page.selected} html={page.content} navigate={navigate} />
      ) : view.kind === "format" ? (
        <FormattedDocument key={page.selected} lines={view.lines} label={`${label} formatted`} />
      ) : view.kind === "source" ? (
        <div className="source-stack">
          {page.previewDisabled && <LargeFileNotice size={page.fileSize} searchExcluded={page.searchExcluded} />}
          {view.formatError && <FormatErrorNotice reason={view.formatError} />}
          <SourceDocument key={page.selected} source={page.source} highlightedLines={page.sourceLines} label={`${label} source`}
            selection={lineSelection} />
        </div>
      ) : hasData ? (
        <div className="empty">
          <h1>Choose a file</h1>
          <p>Select a file from the folder tree.</p>
        </div>
      ) : null}
    </main>
  )
}
