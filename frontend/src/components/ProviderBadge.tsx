import { ReactNode } from "react"

interface ProviderBadgeProps {
  label: string
  variant: "active" | "inactive" | "auth" | "error"
  icon?: ReactNode
}

const VARIANT_STYLES: Record<ProviderBadgeProps["variant"], { bg: string; text: string; dot: string }> = {
  active: {
    bg: "var(--color-green-fill)",
    text: "var(--color-green)",
    dot: "var(--color-green)",
  },
  inactive: {
    bg: "var(--color-surface-2)",
    text: "var(--color-text-tertiary)",
    dot: "var(--color-text-tertiary)",
  },
  auth: {
    bg: "var(--color-orange-fill)",
    text: "var(--color-orange)",
    dot: "var(--color-orange)",
  },
  error: {
    bg: "var(--color-red-fill)",
    text: "var(--color-red)",
    dot: "var(--color-red)",
  },
}

export function ProviderBadge({ label, variant, icon }: ProviderBadgeProps) {
  const colors = VARIANT_STYLES[variant]
  return (
    <span
      style={{
        display: "inline-flex",
        alignItems: "center",
        gap: 6,
        padding: "4px 10px",
        borderRadius: "var(--radius-pill)",
        background: colors.bg,
        color: colors.text,
        fontSize: 11,
        fontWeight: 600,
        letterSpacing: "0.02em",
        whiteSpace: "nowrap",
      }}
    >
      <span
        style={{
          width: 6,
          height: 6,
          borderRadius: "50%",
          background: colors.dot,
          display: "inline-block",
          flexShrink: 0,
        }}
      />
      {icon}
      {label}
    </span>
  )
}
