import { CopyIcon } from "lucide-react"

import { Button } from "./components/ui/button.tsx"
import { useClipboardCopy } from "./clipboard.ts"

interface CodeBlockControlsProps {
  language: string | null
  source: string
}

export function CodeBlockControls({ language, source }: CodeBlockControlsProps) {
  const { status, copy } = useClipboardCopy()
  const description = language ? `${language} code` : "code"

  return (
    <div className="code-block-controls">
      {status !== "idle" && (
        <span className={`code-block-copy-status ${status}`} role="status">
          {status === "copied" ? "Copied" : "Copy failed"}
        </span>
      )}
      <Button
        aria-label={`Copy ${description}`}
        className={`code-block-control h-6 min-w-6 rounded-md px-2${!language || status !== "idle" ? " icon-only" : ""}${status !== "idle" ? " feedback" : ""}`}
        onClick={() => { void copy(source) }}
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
