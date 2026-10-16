import {
  useNavigate,
  useRouter,
  useRouterState,
} from "@tanstack/react-router"
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react"

import { FolderGit2Icon, PanelLeftIcon } from "lucide-react"
import { ProjectPicker, type PickerOpenOptions } from "./ProjectPicker.tsx"
import { DocumentModeToggle, type DocumentMode } from "./DocumentModeToggle.tsx"
import { DocumentPane } from "./DocumentPane.tsx"
import { documentView, resolveDocumentMode } from "./document-view.ts"
import { FileTree } from "./FileTree.tsx"
import { ImageZoomControls } from "./ImageZoomControls.tsx"
import { clampLineRange, lineAnchor, parseLineAnchor, splitSourceLines, type LineRange } from "./line-selection.ts"
import {
  emptyPage,
  isPageLoadResult,
  type PageData,
  type PageLoadResult,
} from "./page-data.ts"
import { SearchDialog } from "./SearchDialog.tsx"
import { SettingsDialog, type OpenSettings } from "./SettingsDialog.tsx"
import { CommandPalette } from "./CommandPalette.tsx"
import type { NavigationKind } from "./code-navigation.ts"
import type { CommandAction } from "./commands.ts"
import type { NavigationLocation } from "./navigation-contract.ts"
import { NavigationDialog } from "./NavigationDialog.tsx"
import type { SourceNavigation } from "./SourceDocument.tsx"
import type { SourceTarget } from "./source-position.ts"
import { useCodeNavigation } from "./use-code-navigation.ts"
import { useNavigationStatus } from "./use-navigation-status.ts"
import { useSettings } from "./components/SettingsProvider.tsx"
import { SidebarResizer } from "./SidebarResizer.tsx"
import { StatusBar } from "./StatusBar.tsx"
import { TabBar } from "./TabBar.tsx"
import { useDelayedFlag, useDocumentFormats } from "./use-document-format.ts"
import { useDocumentNavigation } from "./use-document-navigation.ts"
import { useDocumentWorkspace } from "./use-document-workspace.ts"
import { useImageViewer } from "./use-image-viewer.ts"
import { usePreviewPreference } from "./use-preview-preference.ts"
import { useProjectMemory } from "./use-project-memory.ts"

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
  const lineRange = parseLineAnchor(useRouterState({ select: (state) => state.location.hash }))
  const openSearch = useRef<() => void>(() => {})
  const openFiles = useRef<() => void>(() => {})
  const openProjects = useRef<(options?: PickerOpenOptions) => void>(() => {})
  const openSettings = useRef<OpenSettings>(() => {})
  const openCommands = useRef<() => void>(() => {})
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
  const registerProjects = useCallback((open: ((options?: PickerOpenOptions) => void) | null) => {
    openProjects.current = open ?? (() => {})
  }, [])
  const registerSettings = useCallback((open: OpenSettings | null) => {
    openSettings.current = open ?? (() => {})
  }, [])
  const registerCommands = useCallback((open: (() => void) | null) => {
    openCommands.current = open ?? (() => {})
  }, [])
  const runNavigation = useRef<(kind: NavigationKind) => void>(() => {})
  const runCommand = useCallback((action: CommandAction) => {
    if (action.kind === "go-to-definition") runNavigation.current("definition")
    else if (action.kind === "find-references") runNavigation.current("references")
    else openSettings.current(action.kind === "open-setting" ? action.setting : undefined)
  }, [])
  const chooseProject = useCallback(() => openProjects.current(), [])
  const chooseWorktree = useCallback(() => openProjects.current({ worktrees: true }), [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const workspace = useDocumentWorkspace(result?.kind === "page" ? result.page : null)
  const { setDocumentMode } = workspace
  const previewPreference = usePreviewPreference(project)
  const openRemembered = useCallback((path: string) => {
    void navigate({ to: "/view", search: { project, path }, hash: "", replace: true })
  }, [navigate, project])
  useProjectMemory({
    project,
    page: result?.kind === "page" ? result.page : null,
    tabs: workspace.tabs,
    restore: workspace.restore,
    openRemembered,
  })
  const showSourceAfterFormatFailure = useCallback((path: string) => setDocumentMode(path, "source"), [setDocumentMode])
  const { get: formatResult, request: requestFormat } = useDocumentFormats(project, showSourceAfterFormatFailure)
  // A line anchor (#L12) opens Source unless the tab already chose a view.
  const documentMode = resolveDocumentMode(page, workspace.modes.get(page.selected) ?? (lineRange ? "source" : previewPreference.preferredMode))
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

  const selectLines = useCallback((range: LineRange | null) => {
    if (!page.selected) return
    void navigate({ to: "/view", search: { project, path: page.selected }, hash: range ? lineAnchor(range) : "", replace: true })
  }, [navigate, page.selected, project])
  const showSource = view.kind === "source" && !loadError && !page.error

  // The name last clicked in Source, per file, for F12 and the palette.
  const caret = useRef<{ path: string; target: SourceTarget } | null>(null)
  const openLocation = useCallback((location: NavigationLocation) => {
    caret.current = { path: location.path, target: { position: { line: location.line, column: location.column }, word: null, text: "" } }
    prepareDocumentNavigation()
    void navigate({ to: "/view", search: { project, path: location.path }, hash: lineAnchor({ start: location.line, end: location.line }) })
  }, [navigate, prepareDocumentNavigation, project])
  const languageServers = useSettings().settings.languageServers === "on"
  const navigationStatus = useNavigationStatus(project, page.selected, page.hasFile && page.documentType?.navigable === true, languageServers)
  const codeNavigation = useCodeNavigation(project, languageServers, openLocation, navigationStatus.refresh)
  const { request: requestNavigation, explain: explainNavigation, close: closeNavigation } = codeNavigation
  const openNavigationResult = useCallback((location: NavigationLocation) => {
    closeNavigation()
    openLocation(location)
  }, [closeNavigation, openLocation])
  // Files past the preview limit are not parsed, like they are not highlighted.
  const navigable = showSource && page.documentType?.navigable === true && !page.previewDisabled
  const sourceNavigation = useMemo((): SourceNavigation | undefined => navigable ? {
    onPoint(target) {
      caret.current = { path: page.selected, target }
    },
    onNavigate(kind, target) {
      if (target) caret.current = { path: page.selected, target }
      const remembered = caret.current?.path === page.selected ? caret.current.target : null
      if (remembered) requestNavigation(kind, page.selected, remembered)
      else explainNavigation(kind, "Click a name in the code first, then try again.")
    },
  } : undefined, [explainNavigation, navigable, page.selected, requestNavigation])
  runNavigation.current = (kind) => {
    if (sourceNavigation) sourceNavigation.onNavigate(kind, null)
    else explainNavigation(kind, "Code navigation works in the Source view of supported code files.")
  }
  const selectedLines = showSource && lineRange ? clampLineRange(lineRange, splitSourceLines(page.source).length) : null

  const switchProject = useCallback((next: string) => {
    prepareDocumentNavigation()
    void navigate({ to: "/", search: { project: next }, hash: "" })
  }, [navigate, prepareDocumentNavigation])

  function changeDocumentMode(mode: DocumentMode) {
    if (!page.hasFile || !page.selected || mode === documentMode) return
    if (mode === "format") requestFormat(page.selected, page.source)
    // Line numbers address the original source only.
    if (mode !== "source" && lineRange) selectLines(null)
    setDocumentMode(page.selected, mode)
    previewPreference.choose(mode)
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
          onOpenProjects={chooseProject}
          onOpenWorktrees={chooseWorktree}
          page={page}
          registerOpen={registerFiles}
        />
        <SidebarResizer />
        <section className="workspace" aria-label="Document workspace">
          <div className="workspace-toolbar">
            <button type="button" className="files-toggle" aria-label="Open files" onClick={() => openFiles.current()}>
              <PanelLeftIcon aria-hidden="true" />
            </button>
            <button type="button" className="files-toggle project-toggle" aria-haspopup="dialog"
              aria-label={`Switch project (${page.repository || page.rootName || page.project || "none"})`} onClick={chooseProject}>
              <FolderGit2Icon aria-hidden="true" />
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
            lineSelection={{ path: page.selected, range: lineRange, onChange: selectLines }}
            loadError={loadError}
            main={main}
            navigate={navigateToHref}
            onChooseProject={chooseProject}
            page={page}
            retry={() => { void router.invalidate() }}
            sourceNavigation={sourceNavigation}
            view={view}
          />
        </section>
      </div>
      <StatusBar isLoading={isLoading || formatSlow} error={loadError || page.error} page={page}
        detail={showImage && image.status === "ready" && image.width > 0
          ? `${image.width} × ${image.height} · ${image.percent}%`
          : undefined}
        lines={selectedLines}
        navigation={navigationStatus.status ? {
          status: navigationStatus.status,
          onCheck: navigationStatus.check,
          onOpenSetting: () => openSettings.current("languageServers"),
        } : undefined}
        onSearch={() => openSearch.current()}
        onSettings={() => openSettings.current()}
        onHome={() => {
          prepareDocumentNavigation()
          void navigate({ to: "/", search: { project }, hash: "" })
        }} />
      <SearchDialog project={project} navigate={navigateToDocument} registerOpen={registerSearch} />
      <ProjectPicker project={project} registerOpen={registerProjects} onOpen={switchProject} />
      <SettingsDialog registerOpen={registerSettings} />
      <CommandPalette registerOpen={registerCommands} onRun={runCommand} />
      <NavigationDialog panel={codeNavigation.panel} onClose={closeNavigation} onOpen={openNavigationResult} />
    </>
  )
}
