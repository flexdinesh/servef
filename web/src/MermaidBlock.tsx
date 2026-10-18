import { Component, lazy, Suspense, useEffect, useRef, useState, type ReactNode } from "react"

import { Button } from "./components/ui/button.tsx"
import { useTheme } from "./components/ThemeProvider.tsx"
import { MermaidSource } from "./MermaidSource.tsx"

const ExcalidrawMermaidCanvas = lazy(() => import("./ExcalidrawMermaidCanvas.tsx"))

interface MermaidBoundaryProps {
  children: ReactNode
  source: string
}

interface MermaidBoundaryState {
  failed: boolean
}

class MermaidBoundary extends Component<MermaidBoundaryProps, MermaidBoundaryState> {
  state: MermaidBoundaryState = { failed: false }

  static getDerivedStateFromError(): MermaidBoundaryState {
    return { failed: true }
  }

  render() {
    if (this.state.failed) {
      return (
        <>
          <MermaidSource source={this.props.source} error="Could not load Mermaid. Reload to try again." />
          <Button type="button" variant="outline" onClick={() => window.location.reload()}>
            Reload diagrams
          </Button>
        </>
      )
    }
    return this.props.children
  }
}

export function MermaidBlock({ source }: { source: string }) {
  const host = useRef<HTMLDivElement>(null)
  const [nearViewport, setNearViewport] = useState(false)
  const { resolvedTheme } = useTheme()

  useEffect(() => {
    if (!host.current || typeof IntersectionObserver === "undefined") {
      setNearViewport(true)
      return undefined
    }
    const observer = new IntersectionObserver((entries) => {
      if (!entries.some((entry) => entry.isIntersecting)) return
      setNearViewport(true)
      observer.disconnect()
    }, { rootMargin: "300px 0px" })
    observer.observe(host.current)
    return () => observer.disconnect()
  }, [])

  return (
    <div ref={host} className="mermaid-lazy">
      {nearViewport ? (
        <MermaidBoundary source={source}>
          <Suspense fallback={<MermaidSource source={source} />}>
            <ExcalidrawMermaidCanvas source={source} theme={resolvedTheme} />
          </Suspense>
        </MermaidBoundary>
      ) : <MermaidSource source={source} />}
    </div>
  )
}
