import { cn } from "cn"
import type { HTMLAttributes, ReactNode } from "react"

interface ViewHeaderProps extends Omit<HTMLAttributes<HTMLElement>, "title"> {
  title: ReactNode
  titleId?: string
  // actions sit at the end of the band: icon buttons and segmented controls.
  actions?: ReactNode
  // dialog headers sit on raised surfaces and carry a close action.
  variant?: "view" | "dialog"
}

// ViewHeader is the 32px chrome band that names a sidebar view or dialog.
export function ViewHeader({ title, titleId, actions, variant = "view", className, children, ...props }: ViewHeaderProps) {
  return (
    <header className={cn("ui-view-header", className)} data-variant={variant} {...props}>
      <h2 id={titleId} className="ui-view-title">{title}</h2>
      {children}
      {actions && <div className="ui-view-actions">{actions}</div>}
    </header>
  )
}
