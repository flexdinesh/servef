import { useEffect, useRef, useState } from "react"
import { CopyIcon } from "lucide-react"

import { Button } from "./components/ui/button.tsx"

interface CodeBlockControlsProps {
  language: string | null
  source: string
}

type CopyStatus = "idle" | "copied" | "failed"

function copyWithSelection(source: string): boolean {
  const previousFocus = document.activeElement
  const restoreKeyboardFocus = previousFocus instanceof HTMLElement && previousFocus.matches(":focus-visible")
  const input = document.createElement("textarea")
  input.value = source
  input.setAttribute("readonly", "")
  input.style.position = "fixed"
  input.style.opacity = "0"
  input.style.pointerEvents = "none"
  document.body.append(input)
  input.select()
  try {
    return document.execCommand("copy")
  } finally {
    input.remove()
    if (previousFocus instanceof HTMLElement && restoreKeyboardFocus) previousFocus.focus({ preventScroll: true })
  }
}

async function writeClipboard(source: string): Promise<void> {
  try {
    if (navigator.clipboard) {
      await navigator.clipboard.writeText(source)
      return
    }
  } catch {
    // Plain HTTP and restricted browser contexts can reject the modern API.
  }
  if (!copyWithSelection(source)) throw new Error("clipboard unavailable")
}

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
      await writeClipboard(source)
      setStatus("copied")
    } catch {
      setStatus("failed")
    }
    resetTimer.current = window.setTimeout(() => setStatus("idle"), 1600)
  }

  return (
    <div className="code-block-controls">
      {status !== "idle" && (
        <span className={`code-block-copy-status ${status}`} role="status">
          {status === "copied" ? "Copied" : "Copy failed"}
        </span>
      )}
      <Button
        aria-label={`Copy ${description}`}
        className={`code-block-control h-6 min-w-6 rounded-sm px-1${!language || status !== "idle" ? " icon-only" : ""}${status !== "idle" ? " feedback" : ""}`}
        onClick={copy}
        size="sm"
        type="button"
        variant="outline"
      >
        {language && <span className="code-block-control-language">{language}</span>}
        <CopyIcon
          aria-hidden="true"
          className={`code-block-control-copy size-3${language ? "" : " visible"}`}
        />
      </Button>
    </div>
  )
}
