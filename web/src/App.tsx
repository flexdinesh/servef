import {
  useNavigate,
  useRouter,
  useRouterState,
  useSearch,
} from "@tanstack/react-router"
import {
  Activity,
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
import { splitMinimumWidth } from "./features/source-control/diff-options.ts"
import { ReviewFilesRail, ReviewFilesSheet } from "./features/source-control/ReviewFilesRail.tsx"
import { defaultRailWidth } from "./features/source-control/source-control-layout.ts"
import { onBaseBranch } from "./features/source-control/source-control-panes.ts"
import { useBranchStack } from "./features/source-control/use-branch-stack.ts"
import { useNarrowElement } from "./lib/use-narrow-element.ts"
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
  documentKey, documentTab, includesTab, reviewTab, tabName, tabSearch, walkthroughTab, type WorkspaceTab,
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
import { uiScale } from "./settings/settings.ts"
import { treeRowHeight } from "./components/ui/tree-rows.ts"
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
import { Narration } from "./features/walkthrough/Narration.tsx"
import { WalkthroughsView } from "./features/walkthrough/WalkthroughsView.tsx"
import { WalkthroughTab } from "./features/walkthrough/WalkthroughTab.tsx"
import { WalkthroughStepBar } from "./features/walkthrough/WalkthroughStepBar.tsx"
import { useWalkthroughSession } from "./features/walkthrough/use-walkthrough-session.ts"
import { lineDrift, routeStep, routeWalkthrough, stepTarget, type Drift, type StepRoute } from "./features/walkthrough/walkthrough-steps.ts"
import type { SourceAnnotation } from "./features/documents/SourceDocument.tsx"
import type { ReviewAnnotation } from "./features/source-control/ReviewDocument.tsx"

// routeTab is the tab a route shows: a review, a document (a snapshot with
// at), or a walkthrough's own tab, else none.
function routeTab(review: WorkspaceTab | null, documentPath: string, at: string, walkthrough: string): WorkspaceTab | null {
  if (review) return review
  if (documentPath) return documentTab(documentPath, at)
  return walkthrough ? walkthroughTab(walkthrough) : null
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
  // ?review=branch (&base=, &head=), ?commit=<hash>, or
  // ?from=<hash>&to=<hash>; its page carries only the tree, and #f=<path>
  // names the file in view.
  const searchPath = useSearch({ strict: false, select: (search) => typeof search.path === "string" ? search.path : "" })
  const searchReview = useSearch({ strict: false, select: (search) => search.review === "working" || search.review === "branch" ? search.review : "" })
  const searchBase = useSearch({ strict: false, select: (search) => typeof search.base === "string" ? search.base : "" })
  const searchHead = useSearch({ strict: false, select: (search) => typeof search.head === "string" ? search.head : "" })
  const searchCommit = useSearch({ strict: false, select: (search) => isCommitHash(search.commit) ? search.commit : "" })
  const searchFrom = useSearch({ strict: false, select: (search) => isCommitHash(search.from) ? search.from : "" })
  const searchTo = useSearch({ strict: false, select: (search) => isCommitHash(search.to) ? search.to : "" })
  const reviewRoute = useMemo(() => {
    const compare = routeCompare({
      path: searchPath, review: searchReview, base: searchBase, head: searchHead, commit: searchCommit, from: searchFrom, to: searchTo,
    })
    return compare ? reviewTab(compare) : null
  }, [searchBase, searchCommit, searchFrom, searchHead, searchPath, searchReview, searchTo])
  const reviewCompare = reviewRoute?.kind === "review" ? reviewRoute.compare : null
  // wt=<id> follows a walkthrough, at step=<n>; without a path or review the
  // route is the walkthrough's own tab (decision 0047).
  const searchWalkthrough = useSearch({ strict: false, select: (search) => routeWalkthrough(search.wt) ?? "" })
  const searchStep = useSearch({ strict: false, select: (search) => routeStep(search.step) })
  const walkthroughRoute = !searchPath && !reviewRoute ? searchWalkthrough : ""
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
  const walkthroughCommands = useRef({ move: (_direction: 1 | -1) => {}, close: () => {}, present: () => {} })
  const runCommand = useCallback((action: CommandAction) => {
    if (action.kind === "walkthrough-step") walkthroughCommands.current.move(action.direction)
    else if (action.kind === "walkthrough-exit") walkthroughCommands.current.close()
    else if (action.kind === "walkthrough-present") walkthroughCommands.current.present()
    else if (action.kind === "go-to-definition") runNavigation.current("definition")
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
  const stack = useBranchStack(project, sourceControlActive, sourceControlLayout.base)
  // On the base branch itself, commits list all history (decision 0040).
  const allOnly = onBaseBranch(stack.result?.branch ?? "", stack.result?.base ?? "")
  const commits = useCommits(project, sourceControlActive || reviewLive, allOnly ? "all" : sourceControlLayout.range, sourceControlLayout.base)
  const refs = useRefs(project)
  const chooseProject = useCallback(() => openProjects.current(), [])
  const chooseWorktree = useCallback(() => openProjects.current({ worktrees: true }), [])

  const page = result?.kind === "page" ? result.page : lastPage ?? emptyPage
  const hasData = result?.kind === "page" || lastPage !== null
  const loadError = result?.kind === "failure" ? result.message : ""
  const routedPage = result?.kind === "page" ? result.page : null
  const routedDocument = routedPage?.hasFile ? routedPage.selected : ""
  const routedAt = routedPage?.hasFile ? routedPage.at : ""
  const routedTab = useMemo(() => routeTab(reviewRoute, routedDocument, routedAt, walkthroughRoute),
    [reviewRoute, routedAt, routedDocument, walkthroughRoute])
  const activeDocument = page.hasFile ? page.selected : ""
  const activeAt = page.hasFile ? page.at : ""
  const activeTab = useMemo(() => routeTab(reviewRoute, activeDocument, activeAt, walkthroughRoute),
    [activeAt, activeDocument, reviewRoute, walkthroughRoute])
  // pageKey keys the document's mode, format, and scroll.
  const pageKey = documentKey(page.selected, page.at)
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
  const showSourceAfterFormatFailure = useCallback((key: string) => setDocumentMode(key, "source"), [setDocumentMode])
  const { get: formatResult, request: requestFormat } = useDocumentFormats(project, showSourceAfterFormatFailure)
  // A line anchor (#L12) opens Source unless the tab already chose a view.
  const documentMode = resolveDocumentMode(page, workspace.modes.get(pageKey) ?? (lineRange ? "source" : previewPreference.preferredMode))
  const formatState = formatResult(page.selected, page.source, page.at)
  const view = documentView(page, documentMode, formatState)
  const formatPending = view.kind === "source" && view.formatPending
  const formatSlow = useDelayedFlag(formatPending, 150)
  const image = useImageViewer(page.selected)
  const showImage = view.kind === "image" && !loadError && !page.error
  const needsFormat = documentMode === "format" && formatState === undefined
  useEffect(() => {
    if (needsFormat) requestFormat(page.selected, page.source, page.at)
  }, [needsFormat, page.at, page.selected, page.source, requestFormat])
  const { main, prepareDocumentNavigation, navigateToDocument, navigateToHref,
    forgetDocumentScroll, resetDocumentScroll } = useDocumentNavigation(project, page, isLoading, loadError,
    reviewCompare ? compareKey(reviewCompare) : "")

  // Walkthrough steps open ordinary routes with wt and step added.
  const goStep = useCallback((route: StepRoute, focusDocument: boolean) => {
    if (focusDocument) prepareDocumentNavigation()
    void navigate({ to: "/view", search: { ...route.search }, hash: route.hash })
  }, [navigate, prepareDocumentNavigation])
  const leaveWalkthrough = useRef<() => void>(() => {})
  const leaveStep = useCallback(() => leaveWalkthrough.current(), [])
  const walkthroughs = useWalkthroughSession({
    project, routeId: searchWalkthrough, routeStep: searchStep, go: goStep, leave: leaveStep,
    viewShown: !sidebar.hidden && sidebar.view === "walkthroughs",
  })
  // Present mode hides the workspace chrome around the followed walkthrough;
  // Escape, before anything else, ends it.
  const [presentRequested, setPresentRequested] = useState(false)
  const presenting = presentRequested && walkthroughs.walkthrough !== null
  const { goTo: goToStep, position: stepPosition } = walkthroughs
  const present = useCallback(() => {
    if (!walkthroughs.walkthrough) return
    setPresentRequested(true)
    if (!walkthroughs.routed) goToStep(stepPosition)
  }, [goToStep, stepPosition, walkthroughs.routed, walkthroughs.walkthrough])
  const { close: closeSession } = walkthroughs
  const closeWalkthrough = useCallback(() => {
    setPresentRequested(false)
    closeSession()
  }, [closeSession])
  useEffect(() => {
    if (!presenting) return
    const stop = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || event.defaultPrevented || document.querySelector("dialog[open]")) return
      event.preventDefault()
      event.stopPropagation()
      setPresentRequested(false)
    }
    document.addEventListener("keydown", stop, true)
    return () => document.removeEventListener("keydown", stop, true)
  }, [presenting])
  useLayoutEffect(() => {
    walkthroughCommands.current = { move: walkthroughs.move, close: closeWalkthrough, present }
  })
  // Opening a walkthrough's link shows its steps beside the code on desktop;
  // phones keep the drawer closed.
  const revealedWalkthrough = useRef("")
  const mobileSidebar = sidebar.mobile
  useEffect(() => {
    if (!searchWalkthrough || searchWalkthrough === revealedWalkthrough.current) return
    revealedWalkthrough.current = searchWalkthrough
    if (!mobileSidebar) revealSidebar("walkthroughs")
  }, [mobileSidebar, revealSidebar, searchWalkthrough])
  // Line selections keep the step, so its narration stays.
  const stepSearch = useMemo(() => walkthroughs.routed && searchStep !== undefined
    ? { wt: searchWalkthrough, step: searchStep }
    : {}, [searchStep, searchWalkthrough, walkthroughs.routed])

  // The Commits pane's selection follows an open commit or range review.
  const [commitSelection, setCommitSelection] = useState<Compare | null>(null)
  const [followed, setFollowed] = useState("")
  const reviewKey = reviewCompare ? compareKey(reviewCompare) : ""
  if (reviewKey !== followed) {
    setFollowed(reviewKey)
    if (reviewCompare?.kind === "commit" || reviewCompare?.kind === "range") setCommitSelection(reviewCompare)
  }
  // The last review stays mounted, hidden, while another tab shows, so
  // returning to it keeps its scroll, open files, selection, and rail without
  // rereading its diffs (decision 0048). hash is its last fragment.
  const [keptReview, setKeptReview] = useState<{ project: string; compare: Compare; hash: string } | null>(null)
  if (reviewCompare && !isLoading
    && (keptReview?.project !== project || !sameCompare(keptReview.compare, reviewCompare) || keptReview.hash !== hash)) {
    setKeptReview({ project, compare: reviewCompare, hash })
  }
  const kept = keptReview?.project === project
    && workspace.tabs.some((tab) => tab.kind === "review" && sameCompare(tab.compare, keptReview.compare)) ? keptReview : null
  // shownReview is the active review, else the kept one behind another tab.
  const shownReview = reviewCompare ?? kept?.compare ?? null
  const workingData = useCompareData(project, workingCompare, changes, commits)
  const reviewData = useCompareData(project, shownReview, changes, commits)
  // Commit, range, and branch reviews show their files in a rail beside the
  // diffs; Uncommitted's files are the sidebar's list.
  const reviewRail = reviewCompare !== null && reviewCompare.kind !== "working"
  const shownRail = shownReview !== null && shownReview.kind !== "working"
  const [filesSheet, setFilesSheet] = useState(false)
  // Split needs room in the diff area itself, not just the window.
  const diffNarrow = useNarrowElement(main, splitMinimumWidth) || sidebar.mobile
  const diffOptions = useDiffOptions()
  const viewed = useReviewViewed(project)
  // A pending navigation's location already carries the next route's hash;
  // the review keeps the fragment it last resolved until then.
  const keptShown = kept !== null && shownReview !== null && sameCompare(kept.compare, shownReview)
  const reviewHash = keptShown && (!reviewCompare || isLoading) ? kept.hash : hash
  const fragment = shownReview ? parseReviewFragment(reviewHash) : null
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
    // The kept review returns at the fragment it still shows.
    const keptHash = kept && sameCompare(compare, kept.compare) ? kept.hash : ""
    void navigate({ to: "/view", search: { project, ...compareSearch(compare) }, hash: path ? reviewFragment(path) : keptHash,
      replace: Boolean(reviewCompare && sameCompare(compare, reviewCompare)) })
    if (focusDocument && reviewCompare && sameCompare(compare, reviewCompare)) main.current?.focus({ preventScroll: true })
  }, [kept, main, navigate, prepareDocumentNavigation, project, reviewCompare])
  const selectReviewLines = useCallback((path: string, lines: Parameters<typeof reviewFragment>[1]) => {
    if (!reviewCompare) return
    void navigate({ to: "/view", search: { project, ...compareSearch(reviewCompare), ...stepSearch }, hash: reviewFragment(path, lines), replace: true })
  }, [navigate, project, reviewCompare, stepSearch])
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
  const listedWalkthroughs = walkthroughs.list.result
  const nameTab = useCallback((tab: WorkspaceTab) => tabName(tab, compareContext,
    (id) => listedWalkthroughs?.find((entry) => entry.id === id)?.title ?? ""), [compareContext, listedWalkthroughs])
  const gitStatus = useMemo(() => (changes.result?.files ?? []).map((file) => ({ path: file.path, status: file.status })), [changes.result])
  const { open: openWalkthrough } = walkthroughs
  const selectTab = useCallback((tab: WorkspaceTab) => {
    if (tab.kind === "document") navigateToDocument(tab.path, tab.at)
    else if (tab.kind === "walkthrough") openWalkthrough(tab.id)
    else openReview(tab.compare, true)
  }, [navigateToDocument, openReview, openWalkthrough])

  const selectLines = useCallback((range: LineRange | null) => {
    if (!page.selected) return
    const search = page.at ? { project, path: page.selected, at: page.at } : { project, path: page.selected }
    void navigate({ to: "/view", search: { ...search, ...stepSearch }, hash: range ? lineAnchor(range) : "", replace: true })
  }, [navigate, page.at, page.selected, project, stepSearch])
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
  const { settings } = useSettings()
  const languageServers = settings.languageServers === "on"
  // Snapshots do not navigate: language servers see the working tree.
  const navigationStatus = useNavigationStatus(project, page.selected, page.hasFile && !page.binary && !page.at, languageServers)
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
    if (mode === "format") requestFormat(page.selected, page.source, page.at)
    // Line numbers address the original source only.
    if (mode !== "source" && lineRange) selectLines(null)
    setDocumentMode(pageKey, mode)
    previewPreference.choose(mode)
    resetDocumentScroll(pageKey)
  }

  function closeTab(tab: WorkspaceTab) {
    closeTabs([tab])
  }

  function closeTabs(tabs: readonly WorkspaceTab[]) {
    const closed = workspace.closeTabs(tabs, activeTab)
    if (!closed) return
    for (const tab of closed.tabs) if (tab.kind === "document") forgetDocumentScroll(documentKey(tab.path, tab.at))
    if (!activeTab || !includesTab(closed.tabs, activeTab)) return
    if (closed.replacement) {
      selectTab(closed.replacement)
    } else {
      prepareDocumentNavigation()
      void navigate({ to: "/", search: { project }, hash: "" })
    }
  }

  // Leaving a walkthrough closes its own tab, or keeps the step's file or
  // review open without the narration.
  useLayoutEffect(() => {
    leaveWalkthrough.current = () => {
      if (walkthroughRoute) closeTabs([walkthroughTab(walkthroughRoute)])
      else void navigate({ to: "/view", search: (current) => ({ ...current, wt: undefined, step: undefined }), hash, replace: true })
    }
  })

  // The routed step's narration sits in the code it explains.
  const stepWalkthrough = walkthroughs.routed ? walkthroughs.walkthrough : null
  const stepIndex = walkthroughs.position
  const narrationStep = stepWalkthrough && stepIndex !== null ? stepWalkthrough.steps[stepIndex] : undefined
  const narrationTarget = narrationStep ? stepTarget(narrationStep) : null
  const narrate = (drift: Drift | "missing") => stepWalkthrough && narrationStep && stepIndex !== null
    ? <Narration step={narrationStep} index={stepIndex} count={stepWalkthrough.steps.length} drift={drift}
      canMove={walkthroughs.canMove} onMove={walkthroughs.move} onExit={closeWalkthrough} navigate={navigateToHref} />
    : null
  const sourceAnnotation: SourceAnnotation | undefined = narrationTarget?.kind === "file" && !reviewCompare
    && narrationTarget.path === page.selected && narrationTarget.at === page.at ? {
      line: narrationTarget.lines?.end ?? 0,
      render: (lineAt) => {
        const lines = narrationTarget.lines
        return narrate(lines && lineAt ? lineDrift(narrationTarget.expect, lineAt(lines.start)) : "unknown")
      },
    } : undefined
  const reviewAnnotation: ReviewAnnotation | undefined = narrationTarget?.kind === "diff" && reviewCompare
    && sameCompare(narrationTarget.compare, reviewCompare) ? {
      path: narrationTarget.path,
      side: narrationTarget.side,
      line: narrationTarget.lines?.end ?? 0,
      render: (lineAt) => {
        const lines = narrationTarget.lines
        if (!lineAt) return narrate("missing")
        return narrate(lines ? lineDrift(narrationTarget.expect, lineAt(lines.start)) : "unknown")
      },
    } : undefined

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
      <div className="layout" data-sidebar={sidebar.hidden ? "hidden" : "visible"} data-presenting={presenting || undefined}>
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
          unseenWalkthroughs={walkthroughs.unseen}
          walkthroughs={<WalkthroughsView session={walkthroughs} context={compareContext} mobile={sidebar.mobile} onPresent={present}
            onDismiss={sidebar.dismiss} />}
          sourceControl={<SourceControlView changes={changes} commits={commits} stack={stack} layout={sourceControlLayout} refs={refs}
            working={workingData} workingPath={reviewCompare?.kind === "working" ? currentFile : ""}
            commitSelection={commitSelection} allOnly={allOnly} mobile={sidebar.mobile}
            onDismiss={sidebar.dismiss} onSelectCommits={setCommitSelection} onOpenReview={openReview} />}
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
              <DiffToolbar options={diffOptions.options} narrow={diffNarrow} onChange={diffOptions.change}
                hasChanges={reviewData?.list.kind === "ready" && reviewData.list.files.length > 0}
                anyExpanded={anyFileExpanded} onStepHunk={(direction) => stepHunk.current(direction)}
                onExpandAll={(expand) => setExpandRequest({ expand, id: (expandRequest?.id ?? 0) + 1 })}
                files={reviewRail ? {
                  shown: !sidebar.mobile && !sourceControlLayout.railHidden,
                  onToggle: () => {
                    if (sidebar.mobile) setFilesSheet(true)
                    else sourceControlLayout.setRailHidden(!sourceControlLayout.railHidden)
                  },
                } : undefined} />
            ) : <>
              <DocumentModeToggle page={page} mode={documentMode} busyMode={formatPending ? "format" : undefined}
                onChange={changeDocumentMode} />
              {showImage && <ImageZoomControls viewer={image} />}
            </>}
          </div>
          <div className="workspace-body">
          <DocumentPane
            diff={shownReview ? <ReviewDocument key={compareKey(shownReview)} project={project} compare={shownReview}
              name={nameTab(reviewTab(shownReview))} data={reviewData} options={diffOptions.options} narrow={diffNarrow}
              scrollRoot={main} selection={fragment} expandRequest={expandRequest} treeFiles={treeFiles}
              viewed={viewed.isViewed} onViewedChange={viewed.mark} onSelectLines={selectReviewLines}
              onCurrentFile={setCurrentFile} onStatus={setReviewStatus} onOpenFile={navigateToDocument}
              onAllExpandedChange={setAnyFileExpanded} onRegisterHunkStep={registerHunkStep} annotation={reviewAnnotation} /> : undefined}
            diffShown={reviewCompare !== null}
            walkthrough={walkthroughRoute ? <WalkthroughTab detail={walkthroughs.detail} position={walkthroughs.position}
              context={compareContext} canMove={walkthroughs.canMove} onMove={walkthroughs.move} onGoTo={walkthroughs.goTo}
              onExit={closeWalkthrough} onRetry={walkthroughs.retry} navigate={navigateToHref} /> : undefined}
            sourceAnnotation={sourceAnnotation}
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
          {shownRail && shownReview && !sidebar.mobile && !sourceControlLayout.railHidden && (
            <Activity mode={reviewCompare ? "visible" : "hidden"}>
              <ReviewFilesRail data={reviewData} activePath={currentFile} itemHeight={treeRowHeight(false, uiScale(settings))}
                width={sourceControlLayout.railWidth} onResize={sourceControlLayout.setRailWidth}
                onResetWidth={() => sourceControlLayout.setRailWidth(defaultRailWidth)}
                onHide={() => sourceControlLayout.setRailHidden(true)}
                onOpen={(path, focusDocument) => openReview(shownReview, focusDocument, path)} />
            </Activity>
          )}
          </div>
          {reviewRail && reviewCompare && sidebar.mobile && (
            <ReviewFilesSheet open={filesSheet} data={reviewData} activePath={currentFile} itemHeight={treeRowHeight(true, uiScale(settings))}
              onClose={() => setFilesSheet(false)}
              onOpen={(path, focusDocument) => openReview(reviewCompare, focusDocument, path)} />
          )}
          {walkthroughs.walkthrough && (presenting || (sidebar.mobile && walkthroughs.routed)) && (
            <WalkthroughStepBar walkthrough={walkthroughs.walkthrough} position={walkthroughs.position} presenting={presenting}
              canMove={walkthroughs.canMove} onMove={walkthroughs.move}
              onSteps={sidebar.mobile ? () => revealSidebar("walkthroughs") : undefined}
              onStopPresenting={() => setPresentRequested(false)} />
          )}
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
