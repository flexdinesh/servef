import {
  createRootRoute,
  createRoute,
  createRouter,
} from "@tanstack/react-router"

import { App } from "./App.tsx"
import { loadPage } from "./page-data.ts"

interface PageSearch {
  [key: string]: unknown
  path?: string
}

function validateSearch(search: Record<string, unknown>): PageSearch {
  return {
    ...search,
    path: typeof search.path === "string" ? search.path : undefined,
  }
}

function RoutedApp() {
  return <App />
}

const rootRoute = createRootRoute({
  component: RoutedApp,
})

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  loader: ({ abortController }) => loadPage({
    pathname: "/",
    selected: "",
    signal: abortController.signal,
  }),
})

const viewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "view",
  validateSearch,
  loaderDeps: ({ search }) => ({ selected: search.path ?? "" }),
  loader: ({ abortController, deps }) => loadPage({
    pathname: "/view",
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
