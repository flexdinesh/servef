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
import { defaultProject, fetchDirectories, fetchProjects, landingProject, requestLandingPick } from "./api/projects-data.ts"
import { readMemory } from "./workspace/workspace-memory.ts"
import { isCommitHash } from "./workspace/workspace-tabs.ts"

interface PageSearch {
  [key: string]: unknown
  path?: string
  project?: string
  // diff=1 opens path's working-tree diff instead of the file.
  diff?: 1
  // commit=<hash> opens path's change in that commit instead of the file.
  commit?: string
  // branch=1 opens path's change on this branch since its merge-base.
  branch?: 1
  // from=<hash>&to=<hash> opens path's net change over that commit range.
  from?: string
  to?: string
}

function validateSearch(search: Record<string, unknown>): PageSearch {
  return {
    ...search,
    path: typeof search.path === "string" ? search.path : undefined,
    project: typeof search.project === "string" && search.project !== "" ? search.project : undefined,
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
  // A diff tab of any kind loads only the tree; its pane reads /api/diff.
  loaderDeps: ({ search }) => {
    const diff = search.diff === 1 || search.commit !== undefined || search.branch === 1
      || (search.from !== undefined && search.to !== undefined)
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
