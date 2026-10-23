import { cn } from "cn"
import type { HTMLAttributes } from "react"

interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  // neutral counts results; accent flags activity that needs attention.
  tone?: "neutral" | "accent"
}

export function Badge({ tone = "neutral", className, ...props }: BadgeProps) {
  return <span className={cn("ui-badge", className)} data-tone={tone} {...props} />
}
