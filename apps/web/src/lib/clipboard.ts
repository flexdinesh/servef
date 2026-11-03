import { useCallback, useEffect, useRef, useState } from "react"

export type CopyStatus = "idle" | "copied" | "failed"

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

export async function writeClipboard(source: string): Promise<void> {
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

// useClipboardCopy copies text and reports Copied/Copy failed briefly.
export function useClipboardCopy(resetAfter = 1600) {
  const [status, setStatus] = useState<CopyStatus>("idle")
  const resetTimer = useRef<number | null>(null)

  useEffect(() => () => {
    if (resetTimer.current !== null) window.clearTimeout(resetTimer.current)
  }, [])

  const copy = useCallback(async (text: string) => {
    if (resetTimer.current !== null) window.clearTimeout(resetTimer.current)
    try {
      await writeClipboard(text)
      setStatus("copied")
    } catch {
      setStatus("failed")
    }
    resetTimer.current = window.setTimeout(() => setStatus("idle"), resetAfter)
  }, [resetAfter])

  return { status, copy }
}
