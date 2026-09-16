import { useEffect, useRef, useState } from "react"
import { CheckIcon, CopyIcon, XIcon } from "lucide-react"

import { Button } from "./components/ui/button.tsx"

interface CodeBlockControlsProps {
  language: string | null
  source: string
}

type CopyStatus = "idle" | "copied" | "failed"

export function CodeBlockControls({ language, source }: CodeBlockControlsProps) {
  const [status, setStatus] = useState<CopyStatus>("idle")
  const resetTimer = useRef<number | null>(null)
  const description = language ? `${language} code` : "code"

  useEffect(() => () => {
    if (resetTimer.current !== null) window.clearTimeout(resetTimer.current)
  }, [])

  async function copy() {
    if (resetTimer.current !== null) window.clearTimeout(resetTimer.current)
    try {
      await navigator.clipboard.writeText(source)
      setStatus("copied")
    } catch {
      setStatus("failed")
    }
    resetTimer.current = window.setTimeout(() => setStatus("idle"), 1600)
  }

  const label = status === "copied"
    ? `Copied ${description}`
    : status === "failed"
      ? `Could not copy ${description}`
      : `Copy ${description}`

  return (
    <div className="code-block-controls">
      <Button
        aria-label={label}
        className={`code-block-control h-6 min-w-6 rounded-sm px-1${!language || status !== "idle" ? " icon-only" : ""}`}
        onClick={copy}
        size="sm"
        type="button"
        variant="outline"
      >
        {status === "copied"
          ? <CheckIcon aria-hidden="true" className="size-3" />
          : status === "failed"
            ? <XIcon aria-hidden="true" className="size-3" />
            : (
                <>
                  {language && <span className="code-block-control-language">{language}</span>}
                  <CopyIcon
                    aria-hidden="true"
                    className={`code-block-control-copy size-3${language ? "" : " visible"}`}
                  />
                </>
              )}
      </Button>
    </div>
  )
}
