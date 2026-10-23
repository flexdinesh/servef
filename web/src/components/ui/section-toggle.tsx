import { cn } from "cn"
import { ChevronRightIcon } from "lucide-react"
import type { ButtonHTMLAttributes, ReactNode } from "react"

interface SectionToggleProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-expanded" | "onClick"> {
  expanded: boolean
  onExpandedChange(expanded: boolean): void
  children: ReactNode
}

// SectionToggle collapses a list section. Its chevron is the shared .ui-chevron,
// which rotates inside any [aria-expanded="true"] row.
export function SectionToggle({ expanded, onExpandedChange, children, className, type = "button", ...props }: SectionToggleProps) {
  return (
    <button
      type={type}
      className={cn("ui-section-toggle", className)}
      aria-expanded={expanded}
      onClick={() => onExpandedChange(!expanded)}
      {...props}
    >
      <Chevron />
      {children}
    </button>
  )
}

export function Chevron({ className }: { className?: string }) {
  return <ChevronRightIcon aria-hidden="true" className={cn("ui-chevron", className)} />
}
