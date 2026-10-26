import { cn } from "cn"
import type { ButtonHTMLAttributes, HTMLAttributes, Ref } from "react"

interface SegmentedControlProps extends HTMLAttributes<HTMLDivElement> {
  // sm sits in view headers on a track; md fills the document toolbar as tabs.
  size?: "sm" | "md"
}

// SegmentedControl groups mutually exclusive views or modes on a translucent
// track. The pressed item lifts onto the surface.
export function SegmentedControl({ size = "md", className, role = "group", ...props }: SegmentedControlProps) {
  return <div role={role} className={cn("ui-segmented", className)} data-size={size} {...props} />
}

interface SegmentedItemProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  pressed?: boolean
  ref?: Ref<HTMLButtonElement>
}

// SegmentedItem spreads its props, so it also works as a Base UI render target.
export function SegmentedItem({ pressed, className, type = "button", ref, ...props }: SegmentedItemProps) {
  return <button ref={ref} type={type} aria-pressed={pressed} className={cn("ui-segmented-item", className)} {...props} />
}
