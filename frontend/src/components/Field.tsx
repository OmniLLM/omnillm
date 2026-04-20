import { CSSProperties, ReactNode } from "react"

export const inputStyle: CSSProperties = {
  fontFamily: "var(--font-mono)",
  padding: "6px 10px",
  fontSize: 13,
  background: "var(--color-bg-elevated)",
  border: "1px solid var(--color-border)",
  borderRadius: 6,
  color: "var(--color-text-primary)",
  width: "100%",
  boxSizing: "border-box",
}

export const smallInputStyle: CSSProperties = {
  ...inputStyle,
  fontSize: 11,
  padding: "4px 8px",
  background: "var(--color-bg-elevated)",
}

interface FieldProps {
  label: string
  htmlFor?: string
  children: ReactNode
  description?: string
  error?: string
  required?: boolean
}

export function Field({
  label,
  htmlFor,
  children,
  description,
  error,
  required,
}: FieldProps) {
  return (
    <div style={{ marginBottom: 16 }}>
      <label
        htmlFor={htmlFor}
        className="sys-label"
        style={{
          display: "flex",
          alignItems: "center",
          gap: 2,
          marginBottom: 6,
          fontSize: 12,
          fontWeight: 500,
          color: "var(--color-text-secondary)",
        }}
      >
        {label}
        {required && (
          <span style={{ color: "var(--color-red)", marginLeft: 2 }}>*</span>
        )}
      </label>
      {children}
      {description && !error && (
        <p
          style={{
            fontSize: 11,
            color: "var(--color-text-tertiary)",
            marginTop: 4,
            marginBottom: 0,
            lineHeight: 1.4,
          }}
        >
          {description}
        </p>
      )}
      {error && (
        <p
          role="alert"
          style={{
            fontSize: 11,
            color: "var(--color-red)",
            marginTop: 4,
            marginBottom: 0,
            fontWeight: 500,
            lineHeight: 1.4,
          }}
        >
          {error}
        </p>
      )}
    </div>
  )
}
