import { cn } from "cn"
import type { ButtonHTMLAttributes, MouseEvent, ReactNode, Ref } from "react"

export type IconButtonSize = "xs" | "sm" | "md" | "bar"
export type IconButtonTone = "default" | "success" | "destructive"

type NativeProps = Omit<ButtonHTMLAttributes<HTMLButtonElement>, "aria-label" | "children" | "title">

export interface IconButtonProps extends NativeProps {
  // label names the action for assistive technology and the tooltip.
  label: string
  icon: ReactNode
  // title overrides the tooltip, e.g. to add a shortcut.
  title?: string
  size?: IconButtonSize
  tone?: IconButtonTone
  // disabledReason keeps the control focusable but inert, and explains why.
  disabledReason?: string
  ref?: Ref<HTMLButtonElement>
}

// IconButton is an icon-only action. Sizes: xs 20px (12px glyph) in rows and
// tabs, sm 24px (16px glyph) in headers, toolbars, and fields, md 28px (16px
// glyph) in panels, and bar, a square that fills its band (--bar-size).
export function IconButton({
  label, icon, title, size = "sm", tone = "default", disabledReason, className, type = "button", onClick, ref, ...props
}: IconButtonProps) {
  const inert = Boolean(disabledReason)
  return (
    <button
      ref={ref}
      type={type}
      className={cn("ui-icon-button", className)}
      data-size={size}
      data-tone={tone === "default" ? undefined : tone}
      aria-label={label}
      aria-disabled={inert || undefined}
      title={disabledReason || title || label}
      onClick={(event: MouseEvent<HTMLButtonElement>) => { if (!inert) onClick?.(event) }}
      {...props}
    >
      {icon}
    </button>
  )
}
