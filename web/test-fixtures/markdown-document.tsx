import { StrictMode, useCallback, useRef, useState } from "react"
import { createRoot } from "react-dom/client"

import { FeatureProvider } from "../src/components/FeatureProvider.tsx"
import { ThemeProvider } from "../src/components/ThemeProvider.tsx"
import { DocumentPreview } from "../src/DocumentPreview.tsx"
import type { SearchMatch } from "../src/search-engine.ts"
import { SearchView } from "../src/SearchView.tsx"
import { useSearchSession } from "../src/use-search-session.ts"
import { StatusBar } from "../src/StatusBar.tsx"
import { emptyPage } from "../src/page-data.ts"
import "../src/style.css"

window.EXCALIDRAW_ASSET_PATH = "/assets/excalidraw/"

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
      <FeatureProvider initialFeatures={{ mermaidTldraw: false }}>
        <ThemeProvider initialTheme={{ theme: "light", resolvedTheme: "light" }}>
          <LifecycleFixture />
        </ThemeProvider>
      </FeatureProvider>
    </StrictMode>,
  )
}
