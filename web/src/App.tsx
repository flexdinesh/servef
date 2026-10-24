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
import { IconButton } from "./components/ui/icon-button.tsx"
import { ProjectPicker, type PickerOpenOptions } from "./features/projects/ProjectPicker.tsx"
import { DocumentModeToggle, type DocumentMode } from "./features/documents/DocumentModeToggle.tsx"
import { DocumentPane } from "./features/documents/DocumentPane.tsx"
import { documentView, resolveDocumentMode } from "./features/documents/document-view.ts"
import { FileTree } from "./features/explorer/FileTree.tsx"
import { ImageZoomControls } from "./features/documents/ImageZoomControls.tsx"
import { clampLineRange, lineAnchor, parseLineAnchor, splitSourceLines, type LineRange } from "./lib/line-selection.ts"
import {
  emptyPage,
  isPageLoadResult,
  type PageData,
  type PageLoadResult,
} from "./api/page-data.ts"
import type { SearchMatch } from "./features/search/search-engine.ts"
import { SearchView } from "./features/search/SearchView.tsx"
import { Sidebar } from "./workspace/Sidebar.tsx"
import { SourceControlView } from "./features/source-control/SourceControlView.tsx"
import { ReviewDocument, type HunkStep, type ReviewStatus } from "./features/source-control/ReviewDocument.tsx"
import { DiffToolbar } from "./features/source-control/DiffToolbar.tsx"
import type { CollapseRequest } from "./features/source-control/FilesTree.tsx"
import {
  compareKey, compareSearch, isLiveCompare, parseReviewFragment, reviewFragment, routeCompare, sameCompare, workingCompare,
  type Compare, type CompareContext,
} from "./features/source-control/compare.ts"
import { useChanges } from "./features/source-control/use-changes.ts"
import { useCommits } from "./features/source-control/use-commits.ts"
import { useCompareData } from "./features/source-control/use-compare-data.ts"
import { useDiffOptions } from "./features/source-control/use-diff-options.ts"
import { useRefs } from "./features/source-control/use-refs.ts"
import { useReviewViewed } from "./features/source-control/use-review-viewed.ts"
import { releaseHighlightResources } from "./highlighting/use-highlighted-lines.ts"
import { releaseReviewDiffs } from "./features/source-control/review-diffs.ts"
import { useSourceControlLayout } from "./features/source-control/use-source-control-layout.ts"
import { filePaths } from "./api/page-data.ts"
import { isCommitHash } from "./lib/commit-hash.ts"
import {
  documentTab, includesTab, reviewTab, tabName, tabSearch, type WorkspaceTab,
} from "./workspace/workspace-tabs.ts"
import { SettingsDialog, type OpenSettings } from "./settings/SettingsDialog.tsx"
import { ColorThemePicker } from "./workspace/ColorThemePicker.tsx"
import { CommandPalette } from "./workspace/CommandPalette.tsx"
import { navigationEligibility, type NavigationKind } from "./features/navigation/code-navigation.ts"
import type { CommandAction } from "./workspace/commands.ts"
import type { NavigationLocation } from "./api/navigation-contract.ts"
import { NavigationDialog } from "./features/navigation/NavigationDialog.tsx"
import type { SourceNavigation } from "./features/documents/SourceDocument.tsx"
import type { SourceTarget } from "./features/navigation/source-position.ts"
import { useCodeNavigation } from "./features/navigation/use-code-navigation.ts"
import { useNavigationStatus } from "./features/navigation/use-navigation-status.ts"
import { useSettings } from "./settings/SettingsProvider.tsx"
import { SidebarResizer } from "./workspace/SidebarResizer.tsx"
import { StatusBar, type DiffStatus } from "./workspace/StatusBar.tsx"
import type { CompareData } from "./features/source-control/use-compare-data.ts"
import { TabBar } from "./workspace/TabBar.tsx"
import { useDocumentFormats } from "./features/documents/use-document-format.ts"
import { useDelayedFlag } from "./lib/use-delayed-flag.ts"
import { useDocumentNavigation } from "./features/documents/use-document-navigation.ts"
import { useDocumentWorkspace } from "./workspace/use-document-workspace.ts"
import { useImageViewer } from "./features/documents/use-image-viewer.ts"
import { usePreviewPreference } from "./features/documents/use-preview-preference.ts"
import { useProjectMemory } from "./workspace/use-project-memory.ts"
import { useSearchSession } from "./features/search/use-search-session.ts"
import { useSidebar } from "./workspace/use-sidebar.ts"

// routeTab is the tab a route shows: a review, or a document, else none.
function routeTab(review: WorkspaceTab | null, documentPath: string): WorkspaceTab | null {
  if (review) return review
  return documentPath ? documentTab(documentPath) : null
}

// reviewStatusBar describes the review's file in view, or the one holding
// selected lines.
function reviewStatusBar(status: ReviewStatus | null, currentFile: string, data: CompareData | null): DiffStatus | undefined {
  const path = status?.path ?? currentFile
  if (!path) return undefined
  const stats = data?.stats?.byPath.get(path)
  return {
    path,
    lines: status?.target ?? null,
    stats: stats?.status === "counted" ? { additions: stats.additions, deletions: stats.deletions } : null,
    typeLabel: status?.typeLabel ?? "",
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
  // A review tab routes without a path as /view?review=working,
  // ?review=branch (&base=), ?commit=<hash>, or ?from=<hash>&to=<hash>; its
  // page carries only the tree, and #f=<path> names the file in view.
  const searchPath = useSearch({ strict: false, select: (search) => typeof search.path === "string" ? search.path : "" })
  const searchReview = useSearch({ strict: false, select: (search) => search.review === "working" || search.review === "branch" ? search.review : "" })
  const searchBase = useSearch({ strict: false, select: (search) => typeof search.base === "string" ? search.base : "" })
  const searchCommit = useSearch({ strict: false, select: (search) => isCommitHash(search.commit) ? search.commit : "" })
  const searchFrom = useSearch({ strict: false, select: (search) => isCommitHash(search.from) ? search.from : "" })
  const searchTo = useSearch({ strict: false, select: (search) => isCommitHash(search.to) ? search.to : "" })
  const reviewRoute = useMemo(() => {
    const compare = routeCompare({ path: searchPath, review: searchReview, base: searchBase, commit: searchCommit, from: searchFrom, to: searchTo })
    return compare ? reviewTab(compare) : null
  }, [searchBase, searchCommit, searchFrom, searchPath, searchReview, searchTo])
  const reviewCompare = reviewRoute?.kind === "review" ? reviewRoute.compare : null
  const openSearch = useRef<() => void>(() => {})
  const openFiles = useRef<() => void>(() => {})
  const openProjects = useRef<(options?: PickerOpenOptions) => void>(() => {})
  const openSettings = useRef<OpenSettings>(() => {})
  const openCommands = useRef<() => void>(() => {})
  const openColorTheme = useRef<() => void>(() => {})
  const stepHunk = useRef<HunkStep>(() => {})
  const [lastPage, setLastPage] = useState<PageData | null>(() => result ? initialPage(result) : null)
  useEffect(() => {
    if (result?.kind === "page") setLastPage(result.page)
  }, [result])

  const registerSearch = useCallback((open: (() => void) | null) => {
    openSearch.current = open ?? (() => {})
  }, [])
  const registerHunkStep = useCallback((step: HunkStep | null) => {
    stepHunk.current = step ?? (() => {})
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
  const registerColorTheme = useCallback((open: (() => void) | null) => {
    openColorTheme.current = open ?? (() => {})
  }, [])
  const runNavigation = useRef<(kind: NavigationKind) => void>(() => {})
  const runCommand = useCallback((action: CommandAction) => {
    if (action.kind === "go-to-definition") runNavigation.current("definition")
    else if (action.kind === "find-references") runNavigation.current("references")
    else if (action.kind === "select-color-theme") openColorTheme.current()
    else openSettings.current(action.kind === "open-setting" ? action.setting : undefined)
  }, [])
  const sidebar = useSidebar()
  const { reveal: revealSidebar } = sidebar
  const revealExplorer = useCallback(() => revealSidebar("explorer"), [revealSidebar])
  const revealSearch = useCallback(() => revealSidebar("search"), [revealSidebar])
  const sourceControlActive = !sidebar.hidden && sidebar.view === "source-control"
  // A live review rereads its compare like an open Source Control view.
  const reviewLive = reviewCompare !== null && isLiveCompare(reviewCompare)
  const changes = useChanges(project, sourceControlActive || reviewCompare?.kind === "working")
  const sourceControlLayout = useSourceControlLayout(project)
  const commits = useCommits(project, sourceControlActive || reviewLive, sourceControlLayout.range, sourceControlLayout.base)
  const refs = useRefs(project)
  const chooseProject = useCallback(() => openProjects.current(), [])
  const chooseWorktree = useCallback(() => openProjects.current({ worktrees: true }), [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const routedPage = result?.kind === "page" ? result.page : null
  const routedDocument = routedPage?.hasFile ? routedPage.selected : ""
  const routedTab = useMemo(() => routeTab(reviewRoute, routedDocument), [reviewRoute, routedDocument])
  const activeDocument = page.hasFile ? page.selected : ""
  const activeTab = useMemo(() => routeTab(reviewRoute, activeDocument), [activeDocument, reviewRoute])
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
    forgetDocumentScroll, resetDocumentScroll } = useDocumentNavigation(project, page, isLoading, loadError,
    reviewCompare ? compareKey(reviewCompare) : "")

  // Source Control's selection: the compare its Files pane shows. It follows
  // the open review; until chosen it is the uncommitted changes when there
  // are any, else the branch.
  const [chosen, setChosen] = useState<Compare | null>(null)
  const [followed, setFollowed] = useState("")
  const reviewKey = reviewCompare ? compareKey(reviewCompare) : ""
  if (reviewKey !== followed) {
    setFollowed(reviewKey)
    if (reviewCompare) setChosen(reviewCompare)
  }
  const branchCompare = useMemo((): Compare => ({ kind: "branch", base: sourceControlLayout.base }), [sourceControlLayout.base])
  const selection = chosen ?? (changes.result && changes.result.files.length === 0 ? branchCompare : workingCompare)
  const selectionData = useCompareData(project, selection, changes, commits)
  const reviewData = useCompareData(project, reviewCompare, changes, commits)
  const diffOptions = useDiffOptions()
  const viewed = useReviewViewed(project)
  const fragment = reviewCompare ? parseReviewFragment(hash) : null
  const [currentFile, setCurrentFile] = useState("")
  const [reviewStatus, setReviewStatus] = useState<ReviewStatus | null>(null)
  const [expandRequest, setExpandRequest] = useState<CollapseRequest | null>(null)
  const [anyFileExpanded, setAnyFileExpanded] = useState(true)
  const treeFiles = useMemo(() => new Set(filePaths(page.tree)), [page.tree])

  // openReview opens a compare's review tab; focusDocument moves focus to it.
  const openReview = useCallback((compare: Compare, focusDocument: boolean, path = "") => {
    if (reviewCompare && sameCompare(compare, reviewCompare) && !path) {
      if (focusDocument) main.current?.focus({ preventScroll: true })
      return
    }
    if (focusDocument) prepareDocumentNavigation()
    void navigate({ to: "/view", search: { project, ...compareSearch(compare) }, hash: path ? reviewFragment(path) : "",
      replace: Boolean(reviewCompare && sameCompare(compare, reviewCompare)) })
    if (focusDocument && reviewCompare && sameCompare(compare, reviewCompare)) main.current?.focus({ preventScroll: true })
  }, [main, navigate, prepareDocumentNavigation, project, reviewCompare])
  const selectReviewLines = useCallback((path: string, lines: Parameters<typeof reviewFragment>[1]) => {
    if (!reviewCompare) return
    void navigate({ to: "/view", search: { project, ...compareSearch(reviewCompare) }, hash: reviewFragment(path, lines), replace: true })
  }, [navigate, project, reviewCompare])
  const hasReviewTabs = workspace.tabs.some((tab) => tab.kind === "review")
  // Closing the last review tab frees its workers and caches until the next one.
  useEffect(() => {
    if (!hasReviewTabs) releaseReviewDiffs()
  }, [hasReviewTabs])
  // Closing the last tab frees the Source highlight worker the same way.
  const hasTabs = workspace.tabs.length > 0
  useEffect(() => {
    if (!hasTabs) releaseHighlightResources()
  }, [hasTabs])
  // Branch tabs name the base once Source Control has read it; commit tabs
  // their subject once listed.
  const defaultBase = commits.branches.get("")?.result?.base || (sourceControlLayout.base ? "" : commits.result?.base) || ""
  const listed = commits.result?.commits
  const compareContext = useMemo((): CompareContext => ({
    base: defaultBase,
    subject: (hash) => listed?.find((commit) => commit.hash === hash)?.subject ?? "",
  }), [defaultBase, listed])
  const nameTab = useCallback((tab: WorkspaceTab) => tabName(tab, compareContext), [compareContext])
  const gitStatus = useMemo(() => (changes.result?.files ?? []).map((file) => ({ path: file.path, status: file.status })), [changes.result])
  const selectTab = useCallback((tab: WorkspaceTab) => {
    if (tab.kind === "document") navigateToDocument(tab.path)
    else openReview(tab.compare, true)
  }, [navigateToDocument, openReview])

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
          sourceControl={<SourceControlView changes={changes} commits={commits} layout={sourceControlLayout} refs={refs}
            selection={selection} data={selectionData}
            activePath={reviewCompare && sameCompare(reviewCompare, selection) ? currentFile : ""} mobile={sidebar.mobile}
            onDismiss={sidebar.dismiss} onSelect={setChosen}
            onOpenFile={(path, focusDocument) => openReview(selection, focusDocument, path)}
            onOpenReview={openReview} />}
        />
        <SidebarResizer />
        <section className="workspace" aria-label="Document workspace">
          <div className="workspace-toolbar">
            <IconButton className="workspace-toggle" size="bar" label="Open files"
              icon={<PanelLeftIcon aria-hidden="true" />} onClick={() => openFiles.current()} />
            <IconButton className="workspace-toggle project-toggle" size="bar" aria-haspopup="dialog"
              label={`Switch project (${page.repository || page.rootName || page.project || "none"})`}
              icon={<FolderGit2Icon aria-hidden="true" />} onClick={chooseProject} />
            <TabBar
              active={activeTab}
              tabs={workspace.tabs}
              name={nameTab}
              onClose={closeTab}
              onCloseMany={closeTabs}
              onSelect={selectTab}
            />
            {reviewCompare ? (
              <DiffToolbar options={diffOptions.options} narrow={sidebar.mobile} onChange={diffOptions.change}
                hasChanges={reviewData?.list.kind === "ready" && reviewData.list.files.length > 0}
                anyExpanded={anyFileExpanded} onStepHunk={(direction) => stepHunk.current(direction)}
                onExpandAll={(expand) => setExpandRequest({ expand, id: (expandRequest?.id ?? 0) + 1 })} />
            ) : <>
              <DocumentModeToggle page={page} mode={documentMode} busyMode={formatPending ? "format" : undefined}
                onChange={changeDocumentMode} />
              {showImage && <ImageZoomControls viewer={image} />}
            </>}
          </div>
          <DocumentPane
            diff={reviewCompare ? <ReviewDocument key={compareKey(reviewCompare)} project={project} compare={reviewCompare}
              name={nameTab(reviewTab(reviewCompare))} data={reviewData} options={diffOptions.options} narrow={sidebar.mobile}
              scrollRoot={main} selection={fragment} expandRequest={expandRequest} treeFiles={treeFiles}
              viewed={viewed.isViewed} onViewedChange={viewed.mark} onSelectLines={selectReviewLines}
              onCurrentFile={setCurrentFile} onStatus={setReviewStatus} onOpenFile={navigateToDocument}
              onAllExpandedChange={setAnyFileExpanded} onRegisterHunkStep={registerHunkStep} /> : undefined}
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
        diff={reviewCompare ? reviewStatusBar(reviewStatus, currentFile, reviewData) : undefined}
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
      <ColorThemePicker registerOpen={registerColorTheme} />
      <NavigationDialog panel={codeNavigation.panel} onClose={closeNavigation} onOpen={openNavigationResult} />
    </>
  )
}
