import { cva, type VariantProps } from "class-variance-authority"
import * as React from "react"

import { cn } from "@/lib/utils"

const badgeVariants = cva(
  "inline-flex items-center px-2 py-0.5 text-xs font-semibold rounded-full",
  {
    variants: {
      variant: {
        green: "bg-[var(--color-green-fill)] text-[var(--color-green)]",
        red: "bg-[var(--color-red-fill)] text-[var(--color-red)]",
        yellow: "bg-[var(--color-orange-fill)] text-[var(--color-orange)]",
        blue: "bg-[var(--color-blue-fill)] text-[var(--color-blue)]",
      },
    },
    defaultVariants: { variant: "blue" },
  },
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  )
}

export { Badge, badgeVariants }
