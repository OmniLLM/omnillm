import { useEffect, useId, useState } from "react"

import type { Provider } from "@/api"

export function PriorityModal({
  providers,
  priorities,
  onPrioritiesChange,
}: {
  providers: Array<Provider>
  priorities: Record<string, number>
  onPrioritiesChange: (p: Record<string, number>) => void
}) {
  const [open, setOpen] = useState(false)
  const [ordered, setOrdered] = useState<Array<Provider>>([])
  const titleId = useId()

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [open])

  const openModal = () => {
    const snapshot = providers
      .filter((p) => p.isActive)
      .sort((a, b) => (priorities[a.id] ?? 0) - (priorities[b.id] ?? 0))
    setOrdered(snapshot)
    setOpen(true)
  }

  const activeCount = providers.filter((p) => p.isActive).length
  if (activeCount < 2) return null

  const handleMove = (index: number, direction: -1 | 1) => {
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= ordered.length) return
    setOrdered((prev) => {
      const next = [...prev]
      ;[next[index], next[nextIndex]] = [next[nextIndex], next[index]]
      return next
    })
  }

  const handleSave = () => {
    const newPriorities: Record<string, number> = {}
    for (const [i, p] of ordered.entries()) {
      newPriorities[p.id] = i
    }
    onPrioritiesChange(newPriorities)
    setOpen(false)
  }

  return (
    <>
      <button className="btn btn-ghost btn-sm" onClick={openModal}>
        Priority
      </button>
      {open && (
        <div
          className="dialog-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div
            className="dialog-box"
            style={{ maxWidth: 420 }}
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
          >
            <div className="dialog-header">
              <div
                id={titleId}
                style={{
                  fontFamily: "var(--font-display)",
                  fontWeight: 600,
                  fontSize: 15,
                  color: "var(--color-text)",
                }}
              >
                Routing Priority
              </div>
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => setOpen(false)}
                aria-label="Close dialog"
              >
                ✕
              </button>
            </div>
            <div className="dialog-body">
              <p
                style={{
                  fontSize: 13,
                  color: "var(--color-text-secondary)",
                  marginBottom: 16,
                }}
              >
                Reorder providers. Higher items are tried first.
              </p>
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {ordered.map((p, i) => (
                  <div
                    key={p.id}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 10,
                      padding: "12px 16px",
                      background: "rgba(255,255,255,0.04)",
                      border: "1px solid var(--color-separator)",
                      borderRadius: "var(--radius-md)",
                      transition: "all 0.12s var(--ease)",
                    }}
                  >
                    <span
                      style={{
                        fontFamily: "var(--font-mono)",
                        fontSize: 12,
                        color: "var(--color-blue)",
                        minWidth: 22,
                        fontWeight: 600,
                      }}
                    >
                      {i + 1}
                    </span>
                    <span style={{ flex: 1, fontSize: 14, fontWeight: 500 }}>
                      {p.name}
                    </span>
                    <div
                      style={{ display: "flex", gap: 4, alignItems: "center" }}
                    >
                      <button
                        onClick={() => handleMove(i, -1)}
                        disabled={i === 0}
                        style={{
                          background: "transparent",
                          border: "1px solid var(--color-separator)",
                          borderRadius: 4,
                          color:
                            i === 0 ?
                              "var(--color-text-tertiary)"
                            : "var(--color-text)",
                          cursor: i === 0 ? "not-allowed" : "pointer",
                          opacity: i === 0 ? 0.3 : 1,
                          padding: "2px 8px",
                          fontSize: 14,
                          lineHeight: 1,
                          pointerEvents: i === 0 ? "none" : "auto",
                        }}
                        title="Move up"
                      >
                        ↑
                      </button>
                      <button
                        onClick={() => handleMove(i, 1)}
                        disabled={i === ordered.length - 1}
                        style={{
                          background: "transparent",
                          border: "1px solid var(--color-separator)",
                          borderRadius: 4,
                          color:
                            i === ordered.length - 1 ?
                              "var(--color-text-tertiary)"
                            : "var(--color-text)",
                          cursor:
                            i === ordered.length - 1 ?
                              "not-allowed"
                            : "pointer",
                          opacity: i === ordered.length - 1 ? 0.3 : 1,
                          padding: "2px 8px",
                          fontSize: 14,
                          lineHeight: 1,
                          pointerEvents:
                            i === ordered.length - 1 ? "none" : "auto",
                        }}
                        title="Move down"
                      >
                        ↓
                      </button>
                    </div>
                  </div>
                ))}
              </div>
              <div
                style={{
                  display: "flex",
                  gap: 8,
                  justifyContent: "flex-end",
                  marginTop: 20,
                }}
              >
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => setOpen(false)}
                >
                  Cancel
                </button>
                <button className="btn btn-primary btn-sm" onClick={handleSave}>
                  Save Order
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
