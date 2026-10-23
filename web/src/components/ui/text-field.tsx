import { cn } from "cn"
import type { InputHTMLAttributes, ReactNode, Ref } from "react"

export type FieldVariant = "field" | "palette"

interface TextFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "className"> {
  // className styles the wrapper; the remaining props go to the input.
  className?: string
  inputClassName?: string
  inputRef?: Ref<HTMLInputElement>
  variant?: FieldVariant
  icon?: ReactNode
  // trailing holds counts, toggles, and clear buttons inside the field.
  trailing?: ReactNode
  invalid?: boolean
}

// TextField is a single-line input. field: 28px bordered, for filters and
// forms. palette: borderless 32px with an accent underline, for dialogs.
export function TextField({
  className, inputClassName, inputRef, variant = "field", icon, trailing, invalid, ...props
}: TextFieldProps) {
  return (
    <div className={cn("ui-field", className)} data-variant={variant} data-invalid={invalid || undefined}>
      {icon && <span className="ui-field-icon" aria-hidden="true">{icon}</span>}
      <input ref={inputRef} className={cn("ui-field-input", inputClassName)} aria-invalid={invalid || undefined} {...props} />
      {trailing && <span className="ui-field-trailing">{trailing}</span>}
    </div>
  )
}
