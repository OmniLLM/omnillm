import * as React from "react"

import { cn } from "@/lib/utils"

const Input = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ className, type, ...props }, ref) => (
  <input
    type={type}
    className={cn(
      "w-full bg-transparent border border-[var(--color-separator)] px-3 py-2 text-sm rounded-[var(--radius-md)]",
      "text-[var(--color-text)] placeholder:text-[var(--color-text-tertiary)]",
      "focus:outline-none focus:border-[var(--color-blue)] focus:ring-2 focus:ring-[var(--color-blue-fill)]",
      "transition-colors",
      className,
    )}
    ref={ref}
    {...props}
  />
))
Input.displayName = "Input"

export { Input }
