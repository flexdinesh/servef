import { type MouseEvent, useLayoutEffect, useMemo, useRef, useState } from "react"
import { createPortal } from "react-dom"

import { CodeBlockControls } from "./CodeBlockControls.tsx"
import { MermaidBlock } from "./MermaidBlock.tsx"

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

interface MarkdownDocumentProps {
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

function codeLanguage(code: HTMLElement): string | null {
  if (!code.classList.contains("syntax-highlight")) return null
  const languageClass = [...code.classList].find((className) => className.startsWith("language-"))
  return languageClass?.slice("language-".length) || null
}

export function MarkdownDocument({ html, navigate }: MarkdownDocumentProps) {
  const article = useRef<HTMLElement>(null)
  const [codeBlocks, setCodeBlocks] = useState<CodeBlockPortal[]>([])
  const [diagrams, setDiagrams] = useState<DiagramPortal[]>([])
  const renderedHTML = useMemo(() => ({ __html: html }), [html])

  useLayoutEffect(() => {
    if (!article.current) return
    const nextDiagrams = [...article.current.querySelectorAll("pre > code.language-mermaid")].flatMap((code, index) => {
      const pre = code.parentElement
      if (!pre) return []
      const host = document.createElement("div")
      host.className = "mermaid-portal"
      pre.replaceWith(host)
      return [{ host, key: index, source: code.textContent ?? "" }]
    })
    const nextCodeBlocks = [...article.current.querySelectorAll<HTMLElement>("pre > code")].flatMap((code, index) => {
      const pre = code.parentElement
      if (!pre) return []
      const wrapper = document.createElement("div")
      wrapper.className = "code-block"
      const host = document.createElement("div")
      host.className = "code-block-controls-portal"
      pre.replaceWith(wrapper)
      wrapper.append(pre, host)
      return [{ host, key: index, language: codeLanguage(code), source: code.textContent ?? "" }]
    })
    setDiagrams(nextDiagrams)
    setCodeBlocks(nextCodeBlocks)
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
