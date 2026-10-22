import {
  useNavigate,
  useRouter,
  useRouterState,
  useSearch,
} from "@tanstack/react-router"
import {
  useCallback,
  useEffect,
  useLayoutEffect,
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
import type { SearchMatch } from "./features/search/search-engine.ts"
import { SearchView } from "./features/search/SearchView.tsx"
import { Sidebar } from "./Sidebar.tsx"
import { SourceControlView } from "./features/source-control/SourceControlView.tsx"
import { DiffDocument } from "./features/source-control/DiffDocument.tsx"
import { DiffToolbar } from "./features/source-control/DiffToolbar.tsx"
import { diffStats } from "./features/source-control/diff-stats.ts"
import { useChanges } from "./features/source-control/use-changes.ts"
import { useCommits } from "./features/source-control/use-commits.ts"
import { useDiffTab } from "./features/source-control/use-diff-tab.ts"
import { releaseHighlightResources } from "./use-highlighted-lines.ts"
import { releaseDiffResources } from "./features/source-control/use-parsed-diff.ts"
import { useSourceControlLayout } from "./features/source-control/use-source-control-layout.ts"
import {
  documentTab, includesTab, isCommitHash, routeDiffTab, tabName, tabSearch, type WorkspaceTab,
} from "./workspace-tabs.ts"
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
import { useSearchSession } from "./features/search/use-search-session.ts"
import { useSidebar } from "./use-sidebar.ts"

// routeTab is the tab a route shows: a diff of any kind, or a document, else
// none.
function routeTab(diff: WorkspaceTab | null, documentPath: string): WorkspaceTab | null {
  if (diff) return diff
  return documentPath ? documentTab(documentPath) : null
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
  const hash = useRouterState({ select: (state) => state.location.hash })
  const lineRange = parseLineAnchor(hash)
  // A diff tab routes as /view?path=…&diff=1, a commit diff tab as
  // &commit=<hash>, a branch diff as &branch=1, and a range diff as
  // &from=<hash>&to=<hash>; their page carries only the tree.
  const searchPath = useSearch({ strict: false, select: (search) => typeof search.path === "string" ? search.path : "" })
  const searchDiff = useSearch({ strict: false, select: (search) => search.diff === 1 })
  const searchCommit = useSearch({ strict: false, select: (search) => isCommitHash(search.commit) ? search.commit : "" })
  const searchBranch = useSearch({ strict: false, select: (search) => search.branch === 1 })
  const searchFrom = useSearch({ strict: false, select: (search) => isCommitHash(search.from) ? search.from : "" })
  const searchTo = useSearch({ strict: false, select: (search) => isCommitHash(search.to) ? search.to : "" })
  const diffRoute = useMemo(() => routeDiffTab({
    path: searchPath, diff: searchDiff ? 1 : undefined, commit: searchCommit, branch: searchBranch ? 1 : undefined, from: searchFrom, to: searchTo,
  }), [searchBranch, searchCommit, searchDiff, searchFrom, searchPath, searchTo])
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
  const sourceControlActive = !sidebar.hidden && sidebar.view === "source-control"
  const changes = useChanges(project, sourceControlActive)
  const sourceControlLayout = useSourceControlLayout()
  const commits = useCommits(project, sourceControlActive, sourceControlLayout.range)
  const chooseProject = useCallback(() => openProjects.current(), [])
  const chooseWorktree = useCallback(() => openProjects.current({ worktrees: true }), [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const routedPage = result?.kind === "page" ? result.page : null
  const routedDocument = routedPage?.hasFile ? routedPage.selected : ""
  const routedTab = useMemo(() => routeTab(diffRoute, routedDocument), [diffRoute, routedDocument])
  const activeDocument = page.hasFile ? page.selected : ""
  const activeTab = useMemo(() => routeTab(diffRoute, activeDocument), [activeDocument, diffRoute])
  const workspace = useDocumentWorkspace(routedTab)
  const { setDocumentMode } = workspace
  const previewPreference = usePreviewPreference(project)
  const openRemembered = useCallback((tab: WorkspaceTab) => {
    void navigate({ to: "/view", search: tabSearch(project, tab), hash: "", replace: true })
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

  // The open diff tab and opening diff tabs from Source Control.
  const diff = useDiffTab({
    project, tab: diffRoute, changes, tree: page.tree, hash, narrow: sidebar.mobile, main, prepareDocumentNavigation,
  })
  const { isDeleted, openDiff, openHistoryDiff } = diff
  const hasDiffTabs = workspace.tabs.some((tab) => tab.kind !== "document")
  // Closing the last diff tab frees its workers and caches until the next one.
  useEffect(() => {
    if (!hasDiffTabs) releaseDiffResources()
  }, [hasDiffTabs])
  // Closing the last tab frees the Source highlight worker the same way.
  const hasTabs = workspace.tabs.length > 0
  useEffect(() => {
    if (!hasTabs) releaseHighlightResources()
  }, [hasTabs])
  // Branch tabs name the base once Source Control has read it.
  const branchBase = commits.branch.result?.base || commits.result?.base || ""
  const nameTab = useCallback((tab: WorkspaceTab) => tabName(tab, tab.kind === "diff" && isDeleted(tab.path), branchBase), [branchBase, isDeleted])
  const gitStatus = useMemo(() => (changes.result?.files ?? []).map((file) => ({ path: file.path, status: file.status })), [changes.result])
  const selectTab = useCallback((tab: WorkspaceTab) => {
    if (tab.kind === "document") navigateToDocument(tab.path)
    else if (tab.kind === "diff") openDiff(tab.path, true)
    else openHistoryDiff(tab, true)
  }, [navigateToDocument, openDiff, openHistoryDiff])

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
  // The palette runs navigation against the current document.
  useLayoutEffect(() => {
    runNavigation.current = (kind) => {
      if (sourceNavigation) sourceNavigation.onNavigate(kind, null)
      else explainNavigation(kind, navigationReason)
    }
  }, [explainNavigation, navigationReason, sourceNavigation])
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
          sourceControl={<SourceControlView changes={changes} commits={commits} layout={sourceControlLayout}
            activePath={diff.working ? diff.path : ""} activeHistory={diff.activeHistory} mobile={sidebar.mobile}
            onDismiss={sidebar.dismiss} onOpen={openDiff} onOpenTab={openHistoryDiff} />}
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
            {diff.path ? (
              <DiffToolbar options={diff.options.options} narrow={sidebar.mobile} onChange={diff.options.change}
                openFileReason={diff.openFileReason}
                onOpenFile={() => navigateToDocument(diff.path)} />
            ) : <>
              <DocumentModeToggle page={page} mode={documentMode} busyMode={formatPending ? "format" : undefined}
                onChange={changeDocumentMode} />
              {showImage && <ImageZoomControls viewer={image} />}
            </>}
          </div>
          <DocumentPane
            diff={diff.path ? <DiffDocument content={diff.content} fileDiff={diff.fileDiff} narrow={sidebar.mobile}
              options={diff.options.options} parseFailed={diff.parseFailed} path={diff.path} scrollRoot={main}
              unchangedSince={diffRoute?.kind === "branch" ? branchBase ? `vs ${branchBase}` : "on this branch" : undefined}
              selection={diff.selection} state={diff.state}
              onOpenFile={() => navigateToDocument(diff.path)} onRetry={diff.retry} /> : undefined}
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
      <StatusBar isLoading={isLoading || formatSlow || diff.slow} error={loadError || page.error} page={page}
        diff={diff.path ? {
          path: diff.path,
          lines: diff.target,
          stats: diff.fileDiff ? diffStats(diff.fileDiff) : null,
          typeLabel: diff.state.kind === "ready" ? diff.state.diff.documentType.label : "",
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
