import { useEffect, useId, useRef } from "react"

export interface ConfirmDialogProps {
  open: boolean
  title: string
  message?: string
  confirmLabel?: string
  cancelLabel?: string
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}

/**
 * Accessible confirmation dialog primitive. Replaces native window.confirm()
 * across the app with a properly styled, keyboard-friendly modal.
 *
 * - role="dialog", aria-modal, aria-labelledby
 * - ESC closes; overlay click closes
 * - Initial focus on the cancel button (safer default for destructive flows)
 * - Restores focus to the previously focused element on close
 */
export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  danger = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const titleId = useId()
  const cancelRef = useRef<HTMLButtonElement>(null)
  const previouslyFocused = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (!open) return

    previouslyFocused.current = document.activeElement as HTMLElement | null
    cancelRef.current?.focus()

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation()
        onCancel()
      }
    }
    document.addEventListener("keydown", onKey)
    return () => {
      document.removeEventListener("keydown", onKey)
      previouslyFocused.current?.focus()
    }
  }, [open, onCancel])

  if (!open) return null

  return (
    <div
      className="dialog-overlay"
      onClick={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div
        className="dialog-box"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        style={{ maxWidth: 440 }}
      >
        <div className="dialog-header">
          <div
            id={titleId}
            style={{
              fontFamily: "var(--font-display)",
              fontWeight: 600,
              fontSize: 15,
              color: danger ? "var(--color-red)" : "var(--color-text)",
            }}
          >
            {title}
          </div>
        </div>
        <div className="dialog-body">
          {message && (
            <p
              style={{
                fontSize: 13.5,
                color: "var(--color-text-secondary)",
                lineHeight: 1.55,
                margin: "0 0 18px",
              }}
            >
              {message}
            </p>
          )}
          <div
            style={{
              display: "flex",
              gap: 8,
              justifyContent: "flex-end",
            }}
          >
            <button
              ref={cancelRef}
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={onCancel}
            >
              {cancelLabel}
            </button>
            <button
              type="button"
              className={
                danger ? "btn btn-danger btn-sm" : "btn btn-primary btn-sm"
              }
              onClick={onConfirm}
            >
              {confirmLabel}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}
