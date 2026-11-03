import { cn } from "cn"
import type { HTMLAttributes } from "react"

// Kbd shows a keyboard shortcut.
export function Kbd({ className, ...props }: HTMLAttributes<HTMLElement>) {
  return <kbd className={cn("ui-kbd", className)} {...props} />
}
