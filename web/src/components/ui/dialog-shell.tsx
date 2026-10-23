import { cn } from "cn"
import type { DialogHTMLAttributes, Ref } from "react"

interface DialogShellProps extends DialogHTMLAttributes<HTMLDialogElement> {
  // palette: top-anchored quick pick; panel: centered settings or details;
  // viewer: full-bleed content such as a diagram; drawer: full-height sheet
  // at the inline start, such as the mobile sidebar.
  variant?: "palette" | "panel" | "viewer" | "drawer"
  backdrop?: "transparent" | "overlay"
  ref?: Ref<HTMLDialogElement>
}

// DialogShell styles a native dialog. Callers own opening, closing, and focus
// restore; the shell owns the overlay surface.
export function DialogShell({ variant = "panel", backdrop = "overlay", className, ref, ...props }: DialogShellProps) {
  return (
    <dialog ref={ref} className={cn("ui-dialog", className)} data-variant={variant} data-backdrop={backdrop} {...props} />
  )
}
