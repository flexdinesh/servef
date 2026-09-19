import {
  Link,
  useNavigate,
  useRouter,
  useRouterState,
} from "@tanstack/react-router"
import {
  type KeyboardEvent,
  type PointerEvent,
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
  openDirectoryPaths,
  type PageData,
  type PageLoadResult,
} from "./page-data.ts"
import { SearchDialog } from "./SearchDialog.tsx"
import { StatusBar } from "./StatusBar.tsx"
import { TabBar } from "./TabBar.tsx"
import { useProcessMetrics } from "./process-metrics.ts"

const DEFAULT_SIDEBAR_WIDTH = 288
const MIN_SIDEBAR_WIDTH = 208
const MAX_SIDEBAR_WIDTH = 480
const SIDEBAR_STORAGE_KEY = "servef-sidebar-width"

function clampSidebarWidth(width: number): number {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, width))
}

function readSidebarWidth(): number {
  try {
    const stored = Number(window.localStorage.getItem(SIDEBAR_STORAGE_KEY))
    return Number.isFinite(stored) && stored > 0 ? clampSidebarWidth(stored) : DEFAULT_SIDEBAR_WIDTH
  } catch {
    return DEFAULT_SIDEBAR_WIDTH
  }
}

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
  const main = useRef<HTMLElement>(null)
  const openSearch = useRef<() => void>(() => {})
  const focusDocument = useRef(false)
  const scrollPositions = useRef(new Map<string, number>())
  const resizeStart = useRef<{ pointerID: number, width: number, x: number } | null>(null)
  const [lastPage, setLastPage] = useState<PageData | null>(() => result ? initialPage(result) : null)
  const [openTabs, setOpenTabs] = useState<string[]>(() => {
    const selected = result ? initialPage(result)?.selected : undefined
    return selected ? [selected] : []
  })
  const [documentModes, setDocumentModes] = useState<ReadonlyMap<string, DocumentMode>>(
    () => new Map(),
  )
  const [sidebarWidth, setSidebarWidth] = useState(readSidebarWidth)
  const [expandedPaths, setExpandedPaths] = useState<ReadonlySet<string>>(
    () => new Set(openDirectoryPaths(result ? initialPage(result)?.tree ?? [] : [])),
  )
  const metrics = useProcessMetrics()

  useEffect(() => {
    if (!result || result.kind !== "page") return
    setLastPage(result.page)
    if (result.page.hasFile && result.page.selected) {
      setOpenTabs((current) => current.includes(result.page.selected)
        ? current
        : [...current, result.page.selected])
    }
    const pathsToOpen = openDirectoryPaths(result.page.tree)
    if (pathsToOpen.length === 0) return
    setExpandedPaths((current) => {
      const next = new Set(current)
      let changed = false
      for (const path of pathsToOpen) {
        if (!next.has(path)) {
          next.add(path)
          changed = true
        }
      }
      return changed ? next : current
    })
  }, [result])

  useEffect(() => {
    if (isLoading || !result || result.kind !== "page") return
    if (focusDocument.current) {
      focusDocument.current = false
      main.current?.focus({ preventScroll: true })
    }
    const frame = window.requestAnimationFrame(() => {
      if (main.current) main.current.scrollTop = scrollPositions.current.get(result.page.selected) ?? 0
    })
    return () => { window.cancelAnimationFrame(frame) }
  }, [isLoading, result])

  useEffect(() => {
    document.documentElement.style.setProperty("--sidebar-width", `${sidebarWidth}px`)
    try {
      window.localStorage.setItem(SIDEBAR_STORAGE_KEY, String(sidebarWidth))
    } catch {
      // Resizing still applies when storage is unavailable.
    }
  }, [sidebarWidth])

  const registerSearch = useCallback((open: (() => void) | null) => {
    openSearch.current = open ?? (() => {})
  }, [])

  const saveDocumentScroll = useCallback(() => {
    if (lastPage?.selected && main.current) {
      scrollPositions.current.set(lastPage.selected, main.current.scrollTop)
    }
  }, [lastPage])

  const prepareDocumentNavigation = useCallback(() => {
    saveDocumentScroll()
    focusDocument.current = true
  }, [saveDocumentScroll])

  const navigateToDocument = useCallback((path: string) => {
    prepareDocumentNavigation()
    void navigate({ to: "/view", search: { path }, hash: "" })
  }, [navigate, prepareDocumentNavigation])

  const navigateToHref = useCallback((href: string) => {
    prepareDocumentNavigation()
    void navigate({ href })
  }, [navigate, prepareDocumentNavigation])

  const updateExpanded = useCallback((path: string, expanded: boolean) => {
    setExpandedPaths((current) => {
      if (current.has(path) === expanded) return current
      const next = new Set(current)
      if (expanded) next.add(path)
      else next.delete(path)
      return next
    })
  }, [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const documentMode = documentModes.get(page.selected) ?? "preview"

  const changeDocumentMode = useCallback((mode: DocumentMode) => {
    const path = page.selected
    if (!page.hasFile || !path || mode === documentMode) return
    setDocumentModes((current) => {
      const next = new Map(current)
      next.set(path, mode)
      return next
    })
    scrollPositions.current.set(path, 0)
    if (main.current) main.current.scrollTop = 0
  }, [documentMode, page.hasFile, page.selected])

  const closeTab = useCallback((path: string) => {
    const index = openTabs.indexOf(path)
    if (index < 0) return
    const remaining = openTabs.filter((tab) => tab !== path)
    setOpenTabs(remaining)
    scrollPositions.current.delete(path)
    setDocumentModes((current) => {
      if (!current.has(path)) return current
      const next = new Map(current)
      next.delete(path)
      return next
    })
    if (page.selected !== path) return

    focusDocument.current = true
    const replacement = remaining[Math.min(index, remaining.length - 1)]
    if (replacement) {
      void navigate({ to: "/view", search: { path: replacement }, hash: "" })
    } else {
      void navigate({ to: "/", search: {}, hash: "" })
    }
  }, [navigate, openTabs, page.selected])

  const handleResizeMove = (event: PointerEvent<HTMLDivElement>) => {
    const start = resizeStart.current
    if (!start || start.pointerID !== event.pointerId) return
    setSidebarWidth(clampSidebarWidth(start.width + event.clientX - start.x))
  }

  const stopResize = (event: PointerEvent<HTMLDivElement>) => {
    if (resizeStart.current?.pointerID !== event.pointerId) return
    resizeStart.current = null
    event.currentTarget.releasePointerCapture(event.pointerId)
  }

  const resizeWithKeyboard = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return
    event.preventDefault()
    setSidebarWidth((width) => clampSidebarWidth(width + (event.key === "ArrowLeft" ? -16 : 16)))
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
        <div
          className="sidebar-resizer"
          role="separator"
          aria-label="Resize file tree"
          aria-orientation="vertical"
          aria-valuemin={MIN_SIDEBAR_WIDTH}
          aria-valuemax={MAX_SIDEBAR_WIDTH}
          aria-valuenow={sidebarWidth}
          tabIndex={0}
          onDoubleClick={() => setSidebarWidth(DEFAULT_SIDEBAR_WIDTH)}
          onKeyDown={resizeWithKeyboard}
          onPointerDown={(event) => {
            resizeStart.current = { pointerID: event.pointerId, width: sidebarWidth, x: event.clientX }
            event.currentTarget.setPointerCapture(event.pointerId)
          }}
          onPointerMove={handleResizeMove}
          onPointerUp={stopResize}
          onPointerCancel={stopResize}
        />
        <section className="workspace" aria-label="Document workspace">
          <div className="workspace-toolbar">
            <TabBar
              activePath={page.selected}
              tabs={openTabs}
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
          <StatusBar isLoading={isLoading} metrics={metrics} page={page} />
        </section>
      </div>
      <SearchDialog navigate={navigateToDocument} registerOpen={registerSearch} />
    </>
  )
}
