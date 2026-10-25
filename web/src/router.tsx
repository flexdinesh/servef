import {
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
  useSearch,
} from "@tanstack/react-router"

import { App } from "./App.tsx"
import { isAbort } from "./api/http.ts"
import { loadPage } from "./api/page-data.ts"
import { parseDiffLineAnchor } from "./features/source-control/diff-line-selection.ts"
import { defaultProject, fetchDirectories, fetchProjects, landingProject, requestLandingPick } from "./api/projects-data.ts"
import { readMemory } from "./workspace/workspace-memory.ts"
import { compareSearch, legacyCompare, reviewFragment, routeCompare } from "./features/source-control/compare.ts"
import { isCommitHash } from "./lib/commit-hash.ts"

interface PageSearch {
  [key: string]: unknown
  path?: string
  project?: string
  // Without a path, review=working, review=branch (with base, and head for a
  // stack layer), commit=<hash>, or from=<hash>&to=<hash> open a review tab
  // (decisions 0035, 0040).
  review?: "working" | "branch"
  base?: string
  head?: string
  commit?: string
  from?: string
  to?: string
  // With a path, diff=1, commit=, branch=1, and from=&to= are per-file diff
  // routes from before review tabs; they redirect to the review.
  diff?: 1
  branch?: 1
}

function validateSearch(search: Record<string, unknown>): PageSearch {
  return {
    ...search,
    path: typeof search.path === "string" ? search.path : undefined,
    project: typeof search.project === "string" && search.project !== "" ? search.project : undefined,
    review: search.review === "working" || search.review === "branch" ? search.review : undefined,
    base: typeof search.base === "string" && search.base !== "" ? search.base : undefined,
    head: typeof search.head === "string" && search.head !== "" ? search.head : undefined,
    diff: search.diff === 1 || search.diff === "1" ? 1 : undefined,
    commit: isCommitHash(search.commit) ? search.commit : undefined,
    branch: search.branch === 1 || search.branch === "1" ? 1 : undefined,
    from: isCommitHash(search.from) ? search.from : undefined,
    to: isCommitHash(search.to) ? search.to : undefined,
  }
}

// withProject puts project first so URLs read /view?project=…&path=….
function withProject(search: PageSearch, project: string): PageSearch {
  const { project: _previous, ...rest } = search
  return { project, ...rest }
}

function RoutedApp() {
  // Committed (not pending) search: the workspace switches once the new
  // project's page has loaded, then remounts without the old project's state.
  const project = useSearch({ strict: false, select: (search) => typeof search.project === "string" ? search.project : "" })
  return <App key={project} project={project} />
}

const rootRoute = createRootRoute({
  component: RoutedApp,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  validateSearch,
  // Without a project, reopen the last one, else the first served root. When
  // projects cannot be listed, the page reports that no project is selected.
  beforeLoad: async ({ search, abortController }) => {
    if (search.project) return
    let project = ""
    try {
      const landing = landingProject(await fetchProjects(abortController.signal), readMemory())
      project = landing.fallback
      // An unlisted folder (opened by hand) reopens only while it still exists.
      if (!landing.listed) await fetchDirectories(landing.project, abortController.signal)
      project = landing.project
      if (landing.pick) requestLandingPick()
    } catch (error: unknown) {
      if (isAbort(error)) throw error
    }
    if (project) throw redirect({ to: "/", search: withProject(search, project), replace: true })
  },
  loaderDeps: ({ search }) => ({ project: search.project ?? "" }),
  loader: ({ abortController, deps }) => loadPage({
    pathname: "/",
    project: deps.project,
    selected: "",
    signal: abortController.signal,
  }),
})

const viewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "view",
  validateSearch,
  // A path without a project is relative to the first served root.
  beforeLoad: async ({ search, location, abortController }) => {
    // A per-file diff route opens its review at that file, keeping its lines.
    const legacy = legacyCompare(search)
    if (legacy) {
      const lines = parseDiffLineAnchor(location.hash)
      throw redirect({
        to: "/view",
        search: { ...(search.project ? { project: search.project } : {}), ...compareSearch(legacy.compare) },
        hash: reviewFragment(legacy.path, lines),
        replace: true,
      })
    }
    if (search.project) return
    let project = ""
    try {
      project = defaultProject(await fetchProjects(abortController.signal))
    } catch (error: unknown) {
      if (isAbort(error)) throw error
    }
    // Keep line anchors (#L12) across the default-project redirect.
    if (project) throw redirect({ to: "/view", search: withProject(search, project), hash: location.hash, replace: true })
  },
  // A review tab loads only the tree; its pane reads /api/diff per file.
  loaderDeps: ({ search }) => {
    const diff = routeCompare(search) !== null
    return { project: search.project ?? "", selected: diff ? "" : search.path ?? "", diff }
  },
  loader: ({ abortController, deps }) => loadPage({
    pathname: deps.diff ? "/" : "/view",
    project: deps.project,
    selected: deps.selected,
    signal: abortController.signal,
  }),
})

const routeTree = rootRoute.addChildren([indexRoute, viewRoute])

export const router = createRouter({
  defaultPreload: "intent",
  defaultPreloadStaleTime: 2_000,
  routeTree,
  // The persistent workspace owns document scroll; the tree retains its DOM scroll.
  scrollRestoration: false,
  defaultHashScrollIntoView: false,
})

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}
