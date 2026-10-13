import {
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
  useSearch,
} from "@tanstack/react-router"

import { App } from "./App.tsx"
import { loadPage } from "./page-data.ts"
import { fetchDirectories, fetchProjects, landingProject, rootProject } from "./projects-data.ts"
import { readMemory } from "./workspace-memory.ts"

interface PageSearch {
  [key: string]: unknown
  path?: string
  project?: string
}

function validateSearch(search: Record<string, unknown>): PageSearch {
  return {
    ...search,
    path: typeof search.path === "string" ? search.path : undefined,
    project: typeof search.project === "string" && search.project !== "" ? search.project : undefined,
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
  // Without a project, reopen the last one; /view links stay root-relative.
  beforeLoad: async ({ search, abortController }) => {
    if (search.project) return
    let project = rootProject
    try {
      const landing = landingProject(await fetchProjects(abortController.signal), readMemory())
      // An unlisted folder (opened by hand) reopens only while it still exists.
      if (!landing.listed) await fetchDirectories(landing.project, abortController.signal)
      project = landing.project
    } catch (error: unknown) {
      if (error instanceof Error && error.name === "AbortError") throw error
    }
    throw redirect({ to: "/", search: withProject(search, project), replace: true })
  },
  loaderDeps: ({ search }) => ({ project: search.project ?? rootProject }),
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
  beforeLoad: ({ search }) => {
    if (!search.project) throw redirect({ to: "/view", search: withProject(search, rootProject), replace: true })
  },
  loaderDeps: ({ search }) => ({ project: search.project ?? rootProject, selected: search.path ?? "" }),
  loader: ({ abortController, deps }) => loadPage({
    pathname: "/view",
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
