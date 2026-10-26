import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cn } from "cn"

// plain is ghost without the hover and press layers, for headers whose whole
// band is the target.
export type ButtonVariant = "primary" | "outline" | "ghost" | "plain" | "destructive" | "link"
export type ButtonSize = "sm" | "md" | "lg" | "inline"

type ButtonProps = ButtonPrimitive.Props & {
  variant?: ButtonVariant
  size?: ButtonSize
}

// Button is the text action. Sizes: sm 24px, md 28px (default), lg 32px, and
// inline for text-flow actions such as More/Less; touch layouts raise all but
// inline to 44px.
export function Button({ className, variant = "primary", size = "md", type = "button", ...props }: ButtonProps) {
  return (
    <ButtonPrimitive
      data-slot="button"
      data-variant={variant}
      data-size={size}
      type={type}
      className={cn("ui-button", className)}
      {...props}
    />
  )
}
