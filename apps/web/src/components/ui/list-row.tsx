import { cn } from "cn"
import type { ButtonHTMLAttributes, HTMLAttributes, Ref } from "react"

export type RowDensity = "list" | "pick" | "menu" | "drawer"

interface ListRowProps extends HTMLAttributes<HTMLDivElement> {
  density?: RowDensity
  // active marks the keyboard or pointer cursor in a listbox.
  active?: boolean
  selected?: boolean
  ref?: Ref<HTMLDivElement>
}

// ListRow is one row of a tree, listbox, or result list. Hover adds a state
// layer; active and selected rows take the selection fill and ink.
export function ListRow({ density = "list", active, selected, className, ref, ...props }: ListRowProps) {
  return (
    <div
      ref={ref}
      className={cn("ui-row", className)}
      data-density={density}
      data-active={active || undefined}
      data-selected={selected || undefined}
      {...props}
    />
  )
}

interface ListButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  density?: RowDensity
  selected?: boolean
  ref?: Ref<HTMLButtonElement>
}

// ListButton is a row that performs one action, e.g. a settings category.
export function ListButton({ density = "pick", selected, className, type = "button", ref, ...props }: ListButtonProps) {
  return (
    <button
      ref={ref}
      type={type}
      className={cn("ui-row", className)}
      data-density={density}
      data-selected={selected || undefined}
      {...props}
    />
  )
}
