import { type MouseEvent, useLayoutEffect, useMemo, useRef, useState } from "react"
import { createPortal } from "react-dom"

import { CodeBlockControls } from "./CodeBlockControls.tsx"
import { loadHighlights } from "../../highlighting/highlights-loader.ts"
import { MermaidBlock } from "./MermaidBlock.tsx"
import { highlightLines } from "../../highlighting/syntax-highlight.ts"

interface DiagramPortal {
  host: HTMLDivElement
  key: number
  source: string
}

interface CodeBlockPortal {
  host: HTMLDivElement
  key: number
  language: string | null
  source: string
}

interface DocumentPreviewProps {
  html: string
  navigate(href: string): void
}

function internalDocumentHref(event: MouseEvent<HTMLElement>): string | null {
  if (event.defaultPrevented || event.button !== 0 || event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) {
    return null
  }
  if (!(event.target instanceof Element)) return null
  const anchor = event.target.closest("a")
  if (!(anchor instanceof HTMLAnchorElement) || anchor.hasAttribute("download")) return null
  if (anchor.target && anchor.target !== "_self") return null
  const url = new URL(anchor.href, window.location.href)
  if (url.origin !== window.location.origin || url.pathname !== "/view") return null
  return `${url.pathname}${url.search}${url.hash}`
}

// fenceLanguage is a fence's declared language, from goldmark's class.
function fenceLanguage(code: HTMLElement): string | null {
  const languageClass = [...code.classList].find((className) => className.startsWith("language-"))
  return languageClass?.slice("language-".length) || null
}

export function DocumentPreview({ html, navigate }: DocumentPreviewProps) {
  const article = useRef<HTMLElement>(null)
  const [codeBlocks, setCodeBlocks] = useState<CodeBlockPortal[]>([])
  const [diagrams, setDiagrams] = useState<DiagramPortal[]>([])
  const renderedHTML = useMemo(() => ({ __html: html }), [html])

  useLayoutEffect(() => {
    if (!article.current) return
    const restoreBlocks: Array<() => void> = []
    const nextDiagrams = [...article.current.querySelectorAll("pre > code.language-mermaid")].flatMap((code, index) => {
      const pre = code.parentElement
      if (!pre) return []
      const host = document.createElement("div")
      host.className = "mermaid-portal"
      pre.replaceWith(host)
      restoreBlocks.push(() => host.replaceWith(pre))
      return [{ host, key: index, source: code.textContent ?? "" }]
    })
    const fences: Array<{ code: HTMLElement; language: string | null; source: string }> = []
    const nextCodeBlocks = [...article.current.querySelectorAll<HTMLElement>("pre > code")].flatMap((code, index) => {
      const pre = code.parentElement
      if (!pre) return []
      const wrapper = document.createElement("div")
      wrapper.className = "code-block"
      const host = document.createElement("div")
      host.className = "code-block-controls-portal"
      pre.replaceWith(wrapper)
      wrapper.append(pre, host)
      restoreBlocks.push(() => wrapper.replaceWith(pre))
      const source = code.textContent ?? ""
      fences.push({ code, language: fenceLanguage(code), source })
      // Fences gain a language label once Highlights recognizes it.
      return [{ host, key: index, language: null, source }]
    })
    setDiagrams(nextDiagrams)
    setCodeBlocks(nextCodeBlocks)
    // Fences highlight in the browser (decision 0029); unknown languages and
    // a failed Highlights load stay escaped plain text.
    let current = true
    void loadHighlights().then((highlights) => {
      if (!current || !highlights) return
      const labels = fences.map(({ code, language, source }) => {
        const lines = language ? highlightLines(highlights, source, language) : null
        if (!lines) return null
        code.innerHTML = lines.join("\n")
        return language
      })
      setCodeBlocks((blocks) => blocks.map((block, index) => ({ ...block, language: labels[index] ?? null })))
    })
    return () => {
      current = false
      for (const restore of restoreBlocks) restore()
    }
  }, [html])

  return (
    <>
      <article
        ref={article}
        dangerouslySetInnerHTML={renderedHTML}
        onClick={(event) => {
          const href = internalDocumentHref(event)
          if (!href) return
          event.preventDefault()
          navigate(href)
        }}
      />
      {diagrams.map(({ host, key, source }) => createPortal(<MermaidBlock source={source} />, host, key))}
      {codeBlocks.map(({ host, key, language, source }) => (
        createPortal(<CodeBlockControls language={language} source={source} />, host, key)
      ))}
    </>
  )
}
