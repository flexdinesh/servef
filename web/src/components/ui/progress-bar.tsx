import { cn } from "cn"

interface ProgressBarProps {
  active: boolean
  // edge overlays a divider; page pins to the top of the viewport.
  variant?: "edge" | "page"
  label?: string
  className?: string
}

// ProgressBar is a 2px indeterminate sweep. It appears only after
// --delay-progress so quick work does not flash.
export function ProgressBar({ active, variant = "edge", label = "Loading", className }: ProgressBarProps) {
  return (
    <div
      className={cn("ui-progress", className)}
      data-variant={variant}
      data-active={active || undefined}
      role="progressbar"
      aria-label={label}
      aria-hidden={!active}
    />
  )
}
