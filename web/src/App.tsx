import {
  Link,
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

import { Button } from "@/components/ui/button"

import { ModeToggle } from "./components/ModeToggle.tsx"
import { DocumentModeToggle, type DocumentMode } from "./DocumentModeToggle.tsx"
import { DocumentPane } from "./DocumentPane.tsx"
import { FileTree } from "./FileTree.tsx"
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
import { useDocumentNavigation } from "./use-document-navigation.ts"
import { useDocumentWorkspace } from "./use-document-workspace.ts"
import { useFileTreeExpansion } from "./use-file-tree-expansion.ts"

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

export function App() {
  const navigate = useNavigate()
  const router = useRouter()
  const result = useRouterState({ select: (state) => activePageResult(state.matches) })
  const isLoading = useRouterState({ select: (state) => state.isLoading })
  const openSearch = useRef<() => void>(() => {})
  const [lastPage, setLastPage] = useState<PageData | null>(() => result ? initialPage(result) : null)
  useEffect(() => {
    if (result?.kind === "page") setLastPage(result.page)
  }, [result])

  const registerSearch = useCallback((open: (() => void) | null) => {
    openSearch.current = open ?? (() => {})
  }, [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const workspace = useDocumentWorkspace(result?.kind === "page" ? result.page : null)
  const documentMode = workspace.modes.get(page.selected) ?? "preview"
  const { expandedPaths, updateExpanded } = useFileTreeExpansion(page.tree)
  const { main, prepareDocumentNavigation, navigateToDocument, navigateToHref,
    forgetDocumentScroll, resetDocumentScroll } = useDocumentNavigation(page, isLoading, loadError)

  function changeDocumentMode(mode: DocumentMode) {
    if (!page.hasFile || !page.selected || mode === documentMode) return
    workspace.setDocumentMode(page.selected, mode)
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
      void navigate({ to: "/", search: {}, hash: "" })
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
      <header className="app-header">
        <div className="app-identity">
          <Link className="brand" to="/" search={{}} aria-label="servef home">
            <span className="brand-mark" aria-hidden="true">M</span>
            <span>servef</span>
          </Link>
          {page.rootName && <span className="root-name" title={page.rootName}>{page.rootName}</span>}
        </div>
        <div className="header-actions">
          <Button
            className="search-trigger"
            type="button"
            variant="outline"
            aria-keyshortcuts="Control+K Meta+K"
            onClick={() => openSearch.current()}
          >
            <svg aria-hidden="true" viewBox="0 0 20 20">
              <circle cx="8.5" cy="8.5" r="5.5" />
              <path d="m12.5 12.5 4 4" />
            </svg>
            <span>Search</span>
            <kbd>⌘ K</kbd>
          </Button>
          <ModeToggle />
        </div>
      </header>
      <div className="layout">
        <FileTree
          expandedPaths={expandedPaths}
          hasData={hasData}
          onExpandedChange={updateExpanded}
          onNavigate={prepareDocumentNavigation}
          page={page}
        />
        <SidebarResizer />
        <section className="workspace" aria-label="Document workspace">
          <div className="workspace-toolbar">
            <TabBar
              activePath={page.selected}
              tabs={workspace.tabs}
              onClose={closeTab}
              onSelect={navigateToDocument}
            />
            {page.hasFile && (
              <DocumentModeToggle mode={documentMode} onChange={changeDocumentMode} />
            )}
          </div>
          <DocumentPane
            hasData={hasData}
            isLoading={isLoading}
            loadError={loadError}
            main={main}
            navigate={navigateToHref}
            page={page}
            mode={documentMode}
            retry={() => { void router.invalidate() }}
          />
          <StatusBar isLoading={isLoading} page={page} />
        </section>
      </div>
      <SearchDialog navigate={navigateToDocument} registerOpen={registerSearch} />
    </>
  )
}
