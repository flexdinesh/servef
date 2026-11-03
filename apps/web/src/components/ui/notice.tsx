import { cn } from "cn"
import type { HTMLAttributes } from "react"

export type NoticeTone = "info" | "warning" | "error"

interface NoticeProps extends HTMLAttributes<HTMLDivElement> {
  tone?: NoticeTone
  // bar spans a view edge to edge; block sits inside content.
  layout?: "bar" | "block"
}

// Notice reports a condition that changes what the user can do. Errors are
// alerts; other tones are polite status.
export function Notice({ tone = "info", layout = "block", className, role, ...props }: NoticeProps) {
  return (
    <div
      role={role ?? (tone === "error" ? "alert" : "status")}
      className={cn("ui-notice", className)}
      data-tone={tone}
      data-layout={layout}
      {...props}
    />
  )
}

interface StatusMessageProps extends HTMLAttributes<HTMLParagraphElement> {
  tone?: "muted" | NoticeTone
}

// StatusMessage is quiet supporting text: empty states, counts, and progress.
export function StatusMessage({ tone = "muted", className, ...props }: StatusMessageProps) {
  return <p className={cn("ui-status-message", className)} data-tone={tone} {...props} />
}
