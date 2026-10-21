import {
  useNavigate,
  useRouter,
  useRouterState,
  useSearch,
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
import type { SearchMatch } from "./search-engine.ts"
import { SearchView } from "./SearchView.tsx"
import { Sidebar } from "./Sidebar.tsx"
import { SourceControlView } from "./SourceControlView.tsx"
import { DiffDocument } from "./DiffDocument.tsx"
import { DiffToolbar } from "./DiffToolbar.tsx"
import { diffContent } from "./diff-content.ts"
import { diffStats } from "./diff-stats.ts"
import { useChanges } from "./use-changes.ts"
import { useDiffOptions } from "./use-diff-options.ts"
import { releaseDiffResources, useParsedDiff } from "./use-parsed-diff.ts"
import { useWorkingTreeDiff } from "./use-working-tree-diff.ts"
import { includesTab, tabName, type WorkspaceTab } from "./workspace-tabs.ts"
import { SettingsDialog, type OpenSettings } from "./SettingsDialog.tsx"
import { CommandPalette } from "./CommandPalette.tsx"
import { navigationEligibility, type NavigationKind } from "./code-navigation.ts"
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
import { useSearchSession } from "./use-search-session.ts"
import { useSidebar } from "./use-sidebar.ts"

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
// projects resets transient state, then restores remembered tabs and folders.
export function App({ project }: AppProps) {
  const navigate = useNavigate()
  const router = useRouter()
  const routed = useRouterState({ select: (state) => activePageResult(state.matches) })
  const result = routed?.kind === "page" && routed.page.project !== project ? null : routed
  const isLoading = useRouterState({ select: (state) => state.isLoading })
  const lineRange = parseLineAnchor(useRouterState({ select: (state) => state.location.hash }))
  // A diff tab routes as /view?path=…&diff=1; its page carries only the tree.
  const diffPath = useSearch({ strict: false, select: (search) => search.diff === 1 && typeof search.path === "string" ? search.path : "" })
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
  const sidebar = useSidebar()
  const { reveal: revealSidebar } = sidebar
  const revealExplorer = useCallback(() => revealSidebar("explorer"), [revealSidebar])
  const revealSearch = useCallback(() => revealSidebar("search"), [revealSidebar])
  const changes = useChanges(project, !sidebar.hidden && sidebar.view === "source-control")
  const chooseProject = useCallback(() => openProjects.current(), [])
  const chooseWorktree = useCallback(() => openProjects.current({ worktrees: true }), [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const routedPage = result?.kind === "page" ? result.page : null
  const routedKind = diffPath ? "diff" : routedPage?.hasFile ? "document" : ""
  const routedPath = diffPath || (routedPage?.hasFile ? routedPage.selected : "")
  const routedTab = useMemo((): WorkspaceTab | null => routedKind && routedPath ? { kind: routedKind, path: routedPath } : null, [routedKind, routedPath])
  const activeKind = diffPath ? "diff" : page.hasFile ? "document" : ""
  const activePath = diffPath || (page.hasFile ? page.selected : "")
  const activeTab = useMemo((): WorkspaceTab | null => activeKind && activePath ? { kind: activeKind, path: activePath } : null, [activeKind, activePath])
  const workspace = useDocumentWorkspace(routedTab)
  const { setDocumentMode } = workspace
  const previewPreference = usePreviewPreference(project)
  const openRemembered = useCallback((tab: WorkspaceTab) => {
    void navigate({ to: "/view", search: tab.kind === "diff" ? { project, path: tab.path, diff: 1 } : { project, path: tab.path }, hash: "", replace: true })
  }, [navigate, project])
  useProjectMemory({
    project,
    page: routedPage,
    active: routedTab,
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

  // The open diff tab: its sides, parsed hunks, and toolbar options.
  const diffOptions = useDiffOptions()
  const changedFile = diffPath ? changes.byPath.get(diffPath) : undefined
  const workingDiff = useWorkingTreeDiff(project, diffPath, changedFile?.oldPath ?? "")
  const diffState = workingDiff.state
  const content = useMemo(() => diffState.kind === "ready" ? diffContent(diffState.diff) : null, [diffState])
  const parsedDiff = useParsedDiff(diffPath, content, diffOptions.options.ignoreWhitespace)
  const hasDiffTabs = workspace.tabs.some((tab) => tab.kind === "diff")
  // Closing the last diff tab frees its workers and caches until the next one.
  useEffect(() => {
    if (!hasDiffTabs) releaseDiffResources()
  }, [hasDiffTabs])
  const { fileDiff } = parsedDiff
  const diffSlow = useDelayedFlag(workingDiff.loading || parsedDiff.parsing, 150)
  const diffDeleted = (diffState.kind === "ready" ? diffState.diff.status : changedFile?.status) === "deleted"
  const isDeleted = useCallback((path: string) => changes.byPath.get(path)?.status === "deleted"
    || (path === diffPath && diffDeleted), [changes.byPath, diffDeleted, diffPath])
  const nameTab = useCallback((tab: WorkspaceTab) => tabName(tab, tab.kind === "diff" && isDeleted(tab.path)), [isDeleted])
  const gitStatus = useMemo(() => (changes.result?.files ?? []).map((file) => ({ path: file.path, status: file.status })), [changes.result])

  // openDiff opens a change's diff tab. Click keeps focus in Source Control;
  // Enter and double-click move it to the diff, as in Search.
  const openDiff = useCallback((path: string, focusDocument: boolean) => {
    if (path === diffPath) {
      if (focusDocument) main.current?.focus({ preventScroll: true })
      return
    }
    if (focusDocument) prepareDocumentNavigation()
    void navigate({ to: "/view", search: { project, path, diff: 1 }, hash: "" })
  }, [diffPath, main, navigate, prepareDocumentNavigation, project])
  const selectTab = useCallback((tab: WorkspaceTab) => {
    if (tab.kind === "diff") openDiff(tab.path, true)
    else navigateToDocument(tab.path)
  }, [navigateToDocument, openDiff])

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
  // A search match opens like a navigation location; focus stays in the
  // results unless the user asked to move to the document.
  const openSearchMatch = useCallback((path: string, match: SearchMatch, focusDocument: boolean) => {
    caret.current = { path, target: { position: { line: match.line, column: match.column }, word: null, text: "" } }
    if (focusDocument) prepareDocumentNavigation()
    void navigate({ to: "/view", search: { project, path }, hash: lineAnchor({ start: match.line, end: match.line }) })
  }, [navigate, prepareDocumentNavigation, project])
  const search = useSearchSession({ documentPane: main, openMatch: openSearchMatch, project, registerOpen: registerSearch, reveal: revealSearch })
  const languageServers = useSettings().settings.languageServers === "on"
  const navigationStatus = useNavigationStatus(project, page.selected, page.hasFile && !page.binary, languageServers)
  const codeNavigation = useCodeNavigation(project, languageServers, openLocation, navigationStatus.refresh)
  const { request: requestNavigation, explain: explainNavigation, close: closeNavigation } = codeNavigation
  const openNavigationResult = useCallback((location: NavigationLocation) => {
    closeNavigation()
    openLocation(location)
  }, [closeNavigation, openLocation])
  const navigationReason = navigationEligibility(showSource && documentMode === "source", page.navigationSupported, languageServers, navigationStatus.status)
  const navigable = navigationReason === ""
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
    else explainNavigation(kind, navigationReason)
  }
  useEffect(() => { closeNavigation() }, [closeNavigation, page.selected, documentMode, languageServers])
  useEffect(() => { if (isLoading) closeNavigation() }, [closeNavigation, isLoading])
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

  function closeTab(tab: WorkspaceTab) {
    closeTabs([tab])
  }

  function closeTabs(tabs: readonly WorkspaceTab[]) {
    const closed = workspace.closeTabs(tabs, activeTab)
    if (!closed) return
    for (const tab of closed.tabs) if (tab.kind === "document") forgetDocumentScroll(tab.path)
    if (!activeTab || !includesTab(closed.tabs, activeTab)) return
    if (closed.replacement) {
      selectTab(closed.replacement)
    } else {
      prepareDocumentNavigation()
      void navigate({ to: "/", search: { project }, hash: "" })
    }
  }

  const documentTitle = activeTab ? nameTab(activeTab).title : ""
  useEffect(() => {
    document.title = `${documentTitle ? `${documentTitle} · ` : ""}servef`
  }, [documentTitle])

  return (
    <>
      <div className={`navigation-progress${isLoading ? " active" : ""}`} aria-hidden="true" />
      <div className="visually-hidden" role="status" aria-live="polite">
        {isLoading ? "Loading document…" : documentTitle ? `${documentTitle} loaded.` : ""}
      </div>
      <div className="layout" data-sidebar={sidebar.hidden ? "hidden" : "visible"}>
        <Sidebar
          sidebar={sidebar}
          pendingChanges={changes.result?.files.length ?? 0}
          explorer={<FileTree
            gitStatus={gitStatus}
            hasData={hasData}
            onDismiss={sidebar.dismiss}
            onNavigate={navigateToDocument}
            onOpenProjects={chooseProject}
            onOpenWorktrees={chooseWorktree}
            onReveal={revealExplorer}
            page={page}
            registerOpen={registerFiles}
          />}
          search={<SearchView mobile={sidebar.mobile} onDismiss={sidebar.dismiss}
            onLeave={() => main.current?.focus({ preventScroll: true })} session={search} />}
          sourceControl={<SourceControlView changes={changes} activePath={diffPath} mobile={sidebar.mobile}
            onDismiss={sidebar.dismiss} onOpen={openDiff} />}
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
              active={activeTab}
              tabs={workspace.tabs}
              name={nameTab}
              onClose={closeTab}
              onCloseMany={closeTabs}
              onSelect={selectTab}
            />
            {diffPath ? (
              <DiffToolbar options={diffOptions.options} narrow={sidebar.mobile} onChange={diffOptions.change}
                openFileReason={diffDeleted ? "This file was deleted; there's no working copy to open" : ""}
                onOpenFile={() => navigateToDocument(diffPath)} />
            ) : <>
              <DocumentModeToggle page={page} mode={documentMode} busyMode={formatPending ? "format" : undefined}
                onChange={changeDocumentMode} />
              {showImage && <ImageZoomControls viewer={image} />}
            </>}
          </div>
          <DocumentPane
            diff={diffPath ? <DiffDocument content={content} fileDiff={fileDiff} narrow={sidebar.mobile}
              options={diffOptions.options} parseFailed={parsedDiff.failed} path={diffPath} scrollRoot={main} state={diffState}
              onOpenFile={() => navigateToDocument(diffPath)} onRetry={workingDiff.retry} /> : undefined}
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
      <StatusBar isLoading={isLoading || formatSlow || diffSlow} error={loadError || page.error} page={page}
        diff={diffPath ? {
          path: diffPath,
          stats: fileDiff ? diffStats(fileDiff) : null,
          typeLabel: diffState.kind === "ready" ? diffState.diff.documentType.label : "",
        } : undefined}
        detail={showImage && image.status === "ready" && image.width > 0
          ? `${image.width} × ${image.height} · ${image.percent}%`
          : undefined}
        lines={selectedLines}
        navigation={page.hasFile && !page.binary ? {
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
      <ProjectPicker project={project} registerOpen={registerProjects} onOpen={switchProject} />
      <SettingsDialog registerOpen={registerSettings} />
      <CommandPalette registerOpen={registerCommands} onRun={runCommand} />
      <NavigationDialog panel={codeNavigation.panel} onClose={closeNavigation} onOpen={openNavigationResult} />
    </>
  )
}
