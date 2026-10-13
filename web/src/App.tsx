import {
  useNavigate,
  useRouter,
  useRouterState,
} from "@tanstack/react-router"
import {
  useCallback,
  useEffect,
  useRef,
  useState,
} from "react"

import { PanelLeftIcon } from "lucide-react"
import { DocumentModeToggle, type DocumentMode } from "./DocumentModeToggle.tsx"
import { DocumentPane } from "./DocumentPane.tsx"
import { documentView, resolveDocumentMode } from "./document-view.ts"
import { FileTree } from "./FileTree.tsx"
import { ImageZoomControls } from "./ImageZoomControls.tsx"
import {
  emptyPage,
  isPageLoadResult,
  type PageData,
  type PageLoadResult,
} from "./page-data.ts"
import { SearchDialog } from "./SearchDialog.tsx"
import { SidebarResizer } from "./SidebarResizer.tsx"
import { StatusBar } from "./StatusBar.tsx"
import { TabBar } from "./TabBar.tsx"
import { useDelayedFlag, useDocumentFormats } from "./use-document-format.ts"
import { useDocumentNavigation } from "./use-document-navigation.ts"
import { useDocumentWorkspace } from "./use-document-workspace.ts"
import { useImageViewer } from "./use-image-viewer.ts"

function initialPage(result: PageLoadResult): PageData | null {
  return result.kind === "page" ? result.page : null
}

function activePageResult(matches: readonly { loaderData?: unknown }[]): PageLoadResult | null {
  for (let index = matches.length - 1; index >= 0; index--) {
    const loaderData: unknown = matches[index]?.loaderData
    if (isPageLoadResult(loaderData)) return loaderData
  }
  return null
}

interface AppProps {
  project: string
}

// App is one project's workspace. Mount it keyed by project so switching
// projects discards tabs, scroll, explorer, format, and search state.
export function App({ project }: AppProps) {
  const navigate = useNavigate()
  const router = useRouter()
  const routed = useRouterState({ select: (state) => activePageResult(state.matches) })
  const result = routed?.kind === "page" && routed.page.project !== project ? null : routed
  const isLoading = useRouterState({ select: (state) => state.isLoading })
  const openSearch = useRef<() => void>(() => {})
  const openFiles = useRef<() => void>(() => {})
  const [lastPage, setLastPage] = useState<PageData | null>(() => result ? initialPage(result) : null)
  useEffect(() => {
    if (result?.kind === "page") setLastPage(result.page)
  }, [result])

  const registerSearch = useCallback((open: (() => void) | null) => {
    openSearch.current = open ?? (() => {})
  }, [])
  const registerFiles = useCallback((open: (() => void) | null) => {
    openFiles.current = open ?? (() => {})
  }, [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const workspace = useDocumentWorkspace(result?.kind === "page" ? result.page : null)
  const { setDocumentMode } = workspace
  const showSourceAfterFormatFailure = useCallback((path: string) => setDocumentMode(path, "source"), [setDocumentMode])
  const { get: formatResult, request: requestFormat } = useDocumentFormats(project, showSourceAfterFormatFailure)
  const documentMode = resolveDocumentMode(page, workspace.modes.get(page.selected))
  const formatState = formatResult(page.selected, page.source)
  const view = documentView(page, documentMode, formatState)
  const formatPending = view.kind === "source" && view.formatPending
  const formatSlow = useDelayedFlag(formatPending, 150)
  const image = useImageViewer(page.selected)
  const showImage = view.kind === "image" && !loadError && !page.error
  const needsFormat = documentMode === "format" && formatState === undefined
  useEffect(() => {
    if (needsFormat) requestFormat(page.selected, page.source)
  }, [needsFormat, page.selected, page.source, requestFormat])
  const { main, prepareDocumentNavigation, navigateToDocument, navigateToHref,
    forgetDocumentScroll, resetDocumentScroll } = useDocumentNavigation(project, page, isLoading, loadError)

  function changeDocumentMode(mode: DocumentMode) {
    if (!page.hasFile || !page.selected || mode === documentMode) return
    if (mode === "format") requestFormat(page.selected, page.source)
    setDocumentMode(page.selected, mode)
    resetDocumentScroll(page.selected)
  }

  function closeTab(path: string) {
    const closed = workspace.closeDocument(path)
    if (!closed) return
    forgetDocumentScroll(path)
    if (page.selected !== path) return
    if (closed.replacement) {
      navigateToDocument(closed.replacement)
    } else {
      prepareDocumentNavigation()
      void navigate({ to: "/", search: { project }, hash: "" })
    }
  }

  useEffect(() => {
    document.title = `${page.selected ? `${page.selected} · ` : ""}servef`
  }, [page.selected])

  return (
    <>
      <div className={`navigation-progress${isLoading ? " active" : ""}`} aria-hidden="true" />
      <div className="visually-hidden" role="status" aria-live="polite">
        {isLoading ? "Loading document…" : page.selected ? `${page.selected} loaded.` : ""}
      </div>
      <div className="layout">
        <FileTree
          hasData={hasData}
          onNavigate={navigateToDocument}
          page={page}
          registerOpen={registerFiles}
        />
        <SidebarResizer />
        <section className="workspace" aria-label="Document workspace">
          <div className="workspace-toolbar">
            <button type="button" className="files-toggle" aria-label="Open files" onClick={() => openFiles.current()}>
              <PanelLeftIcon aria-hidden="true" />
            </button>
            <TabBar
              activePath={page.selected}
              tabs={workspace.tabs}
              onClose={closeTab}
              onSelect={navigateToDocument}
            />
            <DocumentModeToggle page={page} mode={documentMode} busyMode={formatPending ? "format" : undefined}
              onChange={changeDocumentMode} />
            {showImage && <ImageZoomControls viewer={image} />}
          </div>
          <DocumentPane
            hasData={hasData}
            image={image}
            isLoading={isLoading}
            loadError={loadError}
            main={main}
            navigate={navigateToHref}
            page={page}
            retry={() => { void router.invalidate() }}
            view={view}
          />
        </section>
      </div>
      <StatusBar isLoading={isLoading || formatSlow} error={loadError || page.error} page={page}
        detail={showImage && image.status === "ready" && image.width > 0
          ? `${image.width} × ${image.height} · ${image.percent}%`
          : undefined}
        onSearch={() => openSearch.current()}
        onHome={() => {
          prepareDocumentNavigation()
          void navigate({ to: "/", search: { project }, hash: "" })
        }} />
      <SearchDialog project={project} navigate={navigateToDocument} registerOpen={registerSearch} />
    </>
  )
}
