import { ReactNode } from "react"

interface SectionProps {
  title: string
  count?: number
  children: ReactNode
  defaultOpen?: boolean
}

export function Section({
  title,
  count,
  children,
  defaultOpen = true,
}: SectionProps) {
  return (
    <div style={{ marginBottom: 12 }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          width: "100%",
          padding: "9px 12px",
          background: "var(--color-surface)",
          border: "1px solid var(--color-separator)",
          borderRadius: 8,
          borderBottomLeftRadius: defaultOpen ? 0 : 8,
          borderBottomRightRadius: defaultOpen ? 0 : 8,
          color: "var(--color-text)",
          fontSize: 12,
          fontWeight: 600,
        }}
      >
        {title}
        {count !== undefined && (
          <span
            style={{
              marginLeft: "auto",
              fontSize: 11,
              background: "var(--color-surface-2)",
              border: "1px solid var(--color-separator)",
              borderRadius: 999,
              padding: "0 7px",
              color: "var(--color-text-tertiary)",
            }}
          >
            {count}
          </span>
        )}
      </div>
      {defaultOpen && (
        <div
          style={{
            padding: 14,
            border: "1px solid var(--color-separator)",
            borderTop: "none",
            borderBottomLeftRadius: 8,
            borderBottomRightRadius: 8,
            background: "var(--color-bg-elevated)",
          }}
        >
          {children}
        </div>
      )}
    </div>
  )
}
