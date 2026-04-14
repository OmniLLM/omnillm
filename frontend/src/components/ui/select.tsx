import * as React from "react"

import { cn } from "@/lib/utils"

const Select = React.forwardRef<
  HTMLSelectElement,
  React.SelectHTMLAttributes<HTMLSelectElement>
>(({ className, children, ...props }, ref) => (
  <select
    ref={ref}
    className={cn(
      "w-full bg-[var(--color-surface-2)] border border-[var(--color-separator)] px-3 py-2 text-sm rounded-[var(--radius-md)]",
      "text-[var(--color-text)] [&>option]:bg-[var(--color-bg-elevated)] [&>option]:text-[var(--color-text)]",
      "focus:outline-none focus:border-[var(--color-blue)] focus:ring-2 focus:ring-[var(--color-blue-fill)]",
      "transition-colors cursor-pointer",
      className,
    )}
    {...props}
  >
    {children}
  </select>
))
Select.displayName = "Select"

export { Select }
