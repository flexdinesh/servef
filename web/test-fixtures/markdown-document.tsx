import { StrictMode, useRef, useState } from "react"
import { createRoot } from "react-dom/client"

import { FeatureProvider } from "../src/components/FeatureProvider.tsx"
import { ThemeProvider } from "../src/components/ThemeProvider.tsx"
import { MarkdownDocument } from "../src/MarkdownDocument.tsx"
import { SearchDialog } from "../src/SearchDialog.tsx"
import { StatusBar } from "../src/StatusBar.tsx"
import { emptyPage } from "../src/page-data.ts"
import "../src/style.css"

window.EXCALIDRAW_ASSET_PATH = "/assets/excalidraw/"

const originalHTML = '<pre><code class="language-mermaid">graph LR\nA --> B</code></pre><pre><code>original code</code></pre>'
const replacementHTML = '<h1>Replacement</h1><pre><code class="language-json syntax-highlight">{"updated":true}</code></pre>'

function LifecycleFixture() {
  const [html, setHTML] = useState(originalHTML)
  const [visible, setVisible] = useState(true)
  const [navigationPrefix, setNavigationPrefix] = useState("original")
  const [navigation, setNavigation] = useState("")
  const openSearch = useRef<(() => void) | null>(null)
  return (
    <>
      <button type="button" onClick={() => setHTML(replacementHTML)}>Update Markdown</button>
      <button type="button" onClick={() => setVisible((current) => !current)}>Toggle document</button>
      <button type="button" onClick={() => setNavigationPrefix("updated")}>Update navigation</button>
      <button type="button" onClick={() => openSearch.current?.()}>Open search</button>
      <output aria-label="Navigation">{navigation}</output>
      {visible && <MarkdownDocument html={html} navigate={() => {}} />}
      {visible && <StatusBar isLoading={false} page={emptyPage} />}
      <SearchDialog
        navigate={(path) => setNavigation(`${navigationPrefix}:${path}`)}
        registerOpen={(open) => { openSearch.current = open }}
      />
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
