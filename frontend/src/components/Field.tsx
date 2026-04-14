import { ReactNode } from "react"

interface FieldProps {
  label: string
  htmlFor?: string
  children: ReactNode
  description?: string
  error?: string
  required?: boolean
}

export function Field({ label, htmlFor, children, description, error, required }: FieldProps) {
  return (
    <div style={{ marginBottom: 16 }}>
      <label
        htmlFor={htmlFor}
        className="sys-label"
        style={{
          display: "block",
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
          }}
        >
          {error}
        </p>
      )}
    </div>
  )
}
