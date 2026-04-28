import type { ReactNode } from "react"

interface MetricCardProps {
  label: string
  value: ReactNode
  icon?: ReactNode
  hint?: string
  accent?: "blue" | "green" | "orange" | "red" | "purple" | "yellow"
}

const ACCENT_VAR: Record<NonNullable<MetricCardProps["accent"]>, string> = {
  blue: "var(--color-blue)",
  green: "var(--color-green)",
  orange: "var(--color-orange)",
  red: "var(--color-red)",
  purple: "var(--color-purple)",
  yellow: "var(--color-yellow)",
}

/**
 * Standardized metric / stat card. Wraps the .stat-card primitive so the
 * many ad-hoc dashboard tiles share the same visual language.
 */
export function MetricCard({
  label,
  value,
  icon,
  hint,
  accent,
}: MetricCardProps) {
  const accentColor = accent ? ACCENT_VAR[accent] : undefined
  return (
    <div
      className="stat-card"
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 6,
        borderTop: accentColor ? `2px solid ${accentColor}` : undefined,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 8,
        }}
      >
        <span className="stat-card-label">{label}</span>
        {icon && (
          <span
            aria-hidden="true"
            style={{ color: accentColor ?? "var(--color-text-tertiary)" }}
          >
            {icon}
          </span>
        )}
      </div>
      <div className="stat-card-value">{value}</div>
      {hint && (
        <div
          style={{
            fontSize: 11,
            color: "var(--color-text-tertiary)",
            marginTop: 2,
          }}
        >
          {hint}
        </div>
      )}
    </div>
  )
}
