import { cn } from "cn"
import { CheckIcon, CopyIcon, XIcon } from "lucide-react"
import type { ReactNode } from "react"

import { useClipboardCopy } from "../../lib/clipboard.ts"

interface CopyButtonProps {
  text: string
  label: string
  // plain sits in rows and bars; raised floats over content; chip carries a
  // visible caption such as a code language.
  variant?: "plain" | "raised" | "chip"
  children?: ReactNode
  className?: string
  iconClassName?: string
}

// CopyButton copies text and confirms or reports failure for --duration-feedback,
// visibly through its icon and audibly through a status announcement.
export function CopyButton({ text, label, variant = "plain", children, className, iconClassName }: CopyButtonProps) {
  const { status, copy } = useClipboardCopy()
  const Icon = status === "copied" ? CheckIcon : status === "failed" ? XIcon : CopyIcon
  return (
    <>
      <button
        type="button"
        className={cn("ui-copy-button", className)}
        data-variant={variant}
        data-status={status}
        aria-label={label}
        title={label}
        onClick={() => { void copy(text) }}
      >
        {children}
        <Icon aria-hidden="true" className={cn("ui-copy-button-icon", iconClassName)} />
      </button>
      <span className="visually-hidden" role="status">
        {status === "copied" ? "Copied" : status === "failed" ? "Copy failed" : ""}
      </span>
    </>
  )
}
