import { ReactNode } from "react"

interface EmptyStateProps {
  icon: ReactNode
  title: string
  description: string
  action?: {
    label: string
    onClick: () => void
  }
}

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        padding: "48px 24px",
        textAlign: "center",
        minHeight: 240,
      }}
    >
      <div
        style={{
          width: 56,
          height: 56,
          borderRadius: "var(--radius-lg)",
          background: "var(--color-surface-2)",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          marginBottom: 16,
          color: "var(--color-text-tertiary)",
        }}
      >
        {icon}
      </div>
      <h3
        style={{
          fontSize: 16,
          fontWeight: 600,
          color: "var(--color-text)",
          margin: "0 0 6px",
          fontFamily: "var(--font-display)",
        }}
      >
        {title}
      </h3>
      <p
        style={{
          fontSize: 14,
          color: "var(--color-text-secondary)",
          margin: "0 0 20px",
          lineHeight: 1.5,
          maxWidth: 320,
        }}
      >
        {description}
      </p>
      {action && (
        <button onClick={action.onClick} className="btn btn-primary">
          {action.label}
        </button>
      )}
    </div>
  )
}
