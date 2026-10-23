import { cn } from "cn"
import type { Ref, SelectHTMLAttributes } from "react"

interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  invalid?: boolean
  ref?: Ref<HTMLSelectElement>
}

// Select is a native select styled as a field (text-field.css).
export function Select({ className, invalid, ref, ...props }: SelectProps) {
  return <select ref={ref} className={cn("ui-select", className)} aria-invalid={invalid || undefined} {...props} />
}
