import { Activity, type ReactNode, type RefObject } from "react"

import { Button } from "@/components/ui/button"

import { BinaryDocument, TooLargeDocument } from "./BinaryDocument.tsx"
import { FormatErrorNotice, LargeFileNotice, SnapshotNotice } from "./DocumentNotice.tsx"
import { DocumentPreview } from "./DocumentPreview.tsx"
import type { DocumentView } from "./document-view.ts"
import { FormattedDocument } from "./FormattedDocument.tsx"
import { handleImageKey, ImageDocument } from "./ImageDocument.tsx"
import { SourceDocument, type LineSelection, type SourceAnnotation, type SourceNavigation } from "./SourceDocument.tsx"
import type { PageData } from "../../api/page-data.ts"
import { projectUrl } from "../../api/project-url.ts"
import type { ImageViewer } from "./use-image-viewer.ts"

interface DocumentPaneProps {
  // diff is a review tab's content, shown instead of the page's file while
  // diffShown; otherwise it stays mounted, hidden, keeping its state.
  diff?: ReactNode
  diffShown?: boolean
  // walkthrough is the open walkthrough tab's content, shown without a file.
  walkthrough?: ReactNode
  // sourceAnnotation is a zone in Source, e.g. a walkthrough's narration.
  sourceAnnotation?: SourceAnnotation
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
  // sourceNavigation is set when Source supports code navigation.
  sourceNavigation?: SourceNavigation
  view: DocumentView
}

function fileName(path: string): string {
  return path.split("/").at(-1) || path
}

export function DocumentPane({
  diff, diffShown = true, hasData, image, isLoading, lineSelection, loadError, main, navigate, onChooseProject, page, retry, sourceAnnotation,
  sourceNavigation, view, walkthrough,
}: DocumentPaneProps) {
  const showDiff = Boolean(diff) && diffShown && !loadError && !page.projectUnavailable && !page.error
  const viewKind = loadError || page.error ? "none" : showDiff ? "diff" : walkthrough ? "walkthrough" : view.kind

  return (
    <main
      className={viewKind === "source" || viewKind === "format" || viewKind === "diff" ? "source-mode" : viewKind === "image" ? "image-mode" : undefined}
      id="document-pane"
      ref={main}
      tabIndex={-1}
      aria-busy={isLoading}
      onKeyDown={viewKind === "image" ? (event) => handleImageKey(event, image) : undefined}
    >
      {/* First, so the diff virtualizer finds it as the pane's content. */}
      {diff && <Activity mode={showDiff ? "visible" : "hidden"}>{diff}</Activity>}
      {showDiff ? null : loadError ? (
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
      ) : walkthrough ? (
        walkthrough
      ) : sourceAnnotation && view.kind !== "source" && view.kind !== "none" ? (
        <div className="annotated-document">
          <div className="document-annotation">{sourceAnnotation.render(null)}</div>
          <PageView hasData={hasData} image={image} lineSelection={lineSelection} navigate={navigate} page={page}
            sourceNavigation={sourceNavigation} view={view} />
        </div>
      ) : (
        <PageView hasData={hasData} image={image} lineSelection={lineSelection} navigate={navigate} page={page}
          sourceAnnotation={sourceAnnotation} sourceNavigation={sourceNavigation} view={view} />
      )}
    </main>
  )
}

interface PageViewProps {
  hasData: boolean
  image: ImageViewer
  lineSelection: LineSelection
  navigate(href: string): void
  page: PageData
  sourceAnnotation?: SourceAnnotation
  sourceNavigation?: SourceNavigation
  view: DocumentView
}

// PageView is the page's file in its chosen view.
function PageView({ hasData, image, lineSelection, navigate, page, sourceAnnotation, sourceNavigation, view }: PageViewProps) {
  const label = page.documentType?.label ?? "File"
  // The server declares the highlight language and the size limit.
  const language = page.highlightDisabled ? null : page.documentType?.language ?? null
  return view.kind === "binary" ? (
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
        page.at ? (
          <div className="source-stack">
            <SnapshotNotice at={page.at} />
            <FormattedDocument key={page.selected} text={view.text} language={language} label={`${label} formatted`} />
          </div>
        ) : <FormattedDocument key={page.selected} text={view.text} language={language} label={`${label} formatted`} />
      ) : view.kind === "source" ? (
        <div className="source-stack">
          {page.at && <SnapshotNotice at={page.at} />}
          {page.previewDisabled && <LargeFileNotice size={page.fileSize} searchExcluded={page.searchExcluded} />}
          {view.formatError && <FormatErrorNotice reason={view.formatError} />}
          <SourceDocument key={page.selected} source={page.source} language={language} label={`${label} source`}
            selection={lineSelection} navigation={sourceNavigation} annotation={sourceAnnotation} />
        </div>
      ) : hasData ? (
        <div className="empty">
          <h1>Choose a file</h1>
          <p>Select a file from the folder tree.</p>
        </div>
      ) : null
}
