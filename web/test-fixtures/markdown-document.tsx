import { StrictMode, useCallback, useRef, useState } from "react"
import { createRoot } from "react-dom/client"

import { ThemeProvider } from "../src/settings/ThemeProvider.tsx"
import { DocumentPreview } from "../src/features/documents/DocumentPreview.tsx"
import type { SearchMatch } from "../src/features/search/search-engine.ts"
import { SearchView } from "../src/features/search/SearchView.tsx"
import { useSearchSession } from "../src/features/search/use-search-session.ts"
import { StatusBar } from "../src/workspace/StatusBar.tsx"
import { emptyPage } from "../src/page-data.ts"
import "../src/style.css"

// Search reads the project named by ?project=, an absolute served path.
const project = new URLSearchParams(window.location.search).get("project") ?? ""

const originalHTML = '<pre><code class="language-mermaid">graph LR\nA --> B</code></pre><pre><code>original code</code></pre>'
const replacementHTML = '<h1>Replacement</h1><pre><code class="language-json syntax-highlight">{"updated":true}</code></pre>'

function LifecycleFixture() {
  const [html, setHTML] = useState(originalHTML)
  const [visible, setVisible] = useState(true)
  const [navigationPrefix, setNavigationPrefix] = useState("original")
  const [navigation, setNavigation] = useState("")
  const openSearch = useRef<(() => void) | null>(null)
  const pane = useRef<HTMLElement>(null)
  const openMatch = useCallback((path: string, match: SearchMatch) => {
    setNavigation(`${navigationPrefix}:${path}:${match.line}`)
  }, [navigationPrefix])
  const registerOpen = useCallback((open: (() => void) | null) => { openSearch.current = open }, [])
  const session = useSearchSession({ documentPane: pane, openMatch, project, registerOpen, reveal: () => {} })
  return (
    <>
      <button type="button" onClick={() => setHTML(replacementHTML)}>Update Markdown</button>
      <button type="button" onClick={() => setVisible((current) => !current)}>Toggle document</button>
      <button type="button" onClick={() => setNavigationPrefix("updated")}>Update navigation</button>
      <button type="button" onClick={() => openSearch.current?.()}>Open search</button>
      <output aria-label="Navigation">{navigation}</output>
      {visible && <DocumentPreview html={html} navigate={() => {}} />}
      {visible && <StatusBar isLoading={false} error="" page={emptyPage} onSearch={() => openSearch.current?.()} onSettings={() => {}} />}
      <SearchView mobile={false} onDismiss={() => {}} onLeave={() => {}} session={session} />
    </>
  )
}

const root = document.getElementById("root")
if (root) {
  createRoot(root).render(
    <StrictMode>
      <ThemeProvider initialTheme={{ theme: "light", resolvedTheme: "light" }}>
        <LifecycleFixture />
      </ThemeProvider>
    </StrictMode>,
  )
}
