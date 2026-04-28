/* eslint-disable @typescript-eslint/no-floating-promises,unicorn/consistent-function-scoping,no-nested-ternary */
import { useEffect, useId, useState } from "react"

import { getProviderUsage, type Provider, type UsageData } from "@/api"
import { Spinner } from "@/components/Spinner"

export function UsageDialog({ provider }: { provider: Provider }) {
  const [data, setData] = useState<UsageData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const titleId = useId()

  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [open])

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setData(await getProviderUsage(provider.id))
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
    if (data === null && !loading) load()
  }

  const getBarColor = (pct: number) =>
    pct > 90 ? "var(--color-red)"
    : pct > 75 ? "var(--color-orange)"
    : "var(--color-green)"

  return (
    <>
      <button className="btn btn-ghost btn-sm" onClick={handleOpen}>
        Usage
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
                {provider.name} — Usage
              </div>
              <div style={{ display: "flex", gap: 8 }}>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={load}
                  disabled={loading}
                >
                  Refresh
                </button>
                <button
                  className="btn btn-ghost btn-sm"
                  onClick={() => setOpen(false)}
                >
                  Done
                </button>
              </div>
            </div>
            <div className="dialog-body">
              {loading && (
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 10,
                    padding: "32px 0",
                    justifyContent: "center",
                    color: "var(--color-text-secondary)",
                    fontSize: 14,
                  }}
                >
                  <Spinner /> Fetching usage data…
                </div>
              )}
              {error && (
                <div
                  style={{
                    color: "var(--color-red)",
                    fontSize: 13,
                    padding: "12px 0",
                  }}
                >
                  {error}
                </div>
              )}
              {data && !loading && (
                <div
                  style={{ display: "flex", flexDirection: "column", gap: 20 }}
                >
                  {(() => {
                    const metaFields = [
                      { label: "Plan", value: data.copilot_plan },
                      { label: "SKU", value: data.access_type_sku },
                      {
                        label: "Quota Resets",
                        value:
                          data.quota_reset_date ?
                            new Date(data.quota_reset_date).toLocaleDateString()
                          : undefined,
                      },
                      {
                        label: "Assigned",
                        value:
                          data.assigned_date ?
                            new Date(data.assigned_date).toLocaleDateString()
                          : undefined,
                      },
                      {
                        label: "Chat",
                        value:
                          data.chat_enabled !== undefined ?
                            data.chat_enabled ?
                              "Enabled"
                            : "Disabled"
                          : undefined,
                      },
                    ].filter((f) => f.value !== undefined)

                    return metaFields.length > 0 ?
                        <div
                          style={{
                            display: "grid",
                            gridTemplateColumns: "1fr 1fr",
                            gap: "12px 24px",
                            padding: "14px 16px",
                            background: "rgba(255,255,255,0.04)",
                            borderRadius: "var(--radius-md)",
                            border: "1px solid var(--color-separator)",
                          }}
                        >
                          {metaFields.map(({ label, value }) => (
                            <div key={label}>
                              <div
                                style={{
                                  fontSize: 11,
                                  color: "var(--color-text-tertiary)",
                                  marginBottom: 3,
                                }}
                              >
                                {label}
                              </div>
                              <div
                                style={{
                                  fontSize: 13,
                                  color: "var(--color-text)",
                                  textTransform: "capitalize",
                                }}
                              >
                                {value}
                              </div>
                            </div>
                          ))}
                        </div>
                      : null
                  })()}

                  {(
                    data.quota_snapshots
                    && Object.keys(data.quota_snapshots).length > 0
                  ) ?
                    <div
                      style={{
                        display: "flex",
                        flexDirection: "column",
                        gap: 18,
                      }}
                    >
                      {Object.entries(data.quota_snapshots).map(
                        ([key, value]) => {
                          const { entitlement, percent_remaining, unlimited } =
                            value
                          const remaining =
                            value.quota_remaining ?? value.remaining
                          const pctUsed =
                            unlimited ? 0 : 100 - percent_remaining
                          const used =
                            unlimited ? "N/A" : (
                              (entitlement - remaining).toLocaleString()
                            )
                          const barColor =
                            unlimited ? "var(--color-blue)" : (
                              getBarColor(pctUsed)
                            )
                          return (
                            <div key={key}>
                              <div
                                style={{
                                  display: "flex",
                                  justifyContent: "space-between",
                                  marginBottom: 8,
                                  alignItems: "baseline",
                                }}
                              >
                                <span
                                  style={{
                                    fontSize: 13,
                                    fontWeight: 500,
                                    textTransform: "capitalize",
                                    color: "var(--color-text)",
                                  }}
                                >
                                  {key.replaceAll("_", " ")}
                                </span>
                                {unlimited ?
                                  <span
                                    style={{
                                      fontSize: 11,
                                      padding: "2px 8px",
                                      background: "var(--color-blue-fill)",
                                      borderRadius: "var(--radius-pill)",
                                      color: "var(--color-blue)",
                                      fontWeight: 500,
                                    }}
                                  >
                                    Unlimited
                                  </span>
                                : <span
                                    style={{
                                      fontSize: 12,
                                      fontFamily: "var(--font-mono)",
                                      color:
                                        pctUsed > 75 ? barColor : (
                                          "var(--color-text-secondary)"
                                        ),
                                    }}
                                  >
                                    {pctUsed.toFixed(1)}% used
                                  </span>
                                }
                              </div>
                              <div className="progress-track">
                                <div
                                  className="progress-bar"
                                  style={{
                                    width: `${unlimited ? 100 : pctUsed}%`,
                                    background: barColor,
                                  }}
                                />
                              </div>
                              <div
                                style={{
                                  display: "flex",
                                  justifyContent: "space-between",
                                  marginTop: 5,
                                  fontSize: 12,
                                  color: "var(--color-text-secondary)",
                                  fontFamily: "var(--font-mono)",
                                }}
                              >
                                <span>
                                  {used} /{" "}
                                  {unlimited ?
                                    "∞"
                                  : entitlement.toLocaleString()}
                                </span>
                                <span>
                                  {unlimited ? "∞" : remaining.toLocaleString()}{" "}
                                  remaining
                                </span>
                              </div>
                            </div>
                          )
                        },
                      )}
                    </div>
                  : <div>
                      <div
                        style={{
                          fontSize: 12,
                          color: "var(--color-text-secondary)",
                          marginBottom: 10,
                        }}
                      >
                        Raw Data
                      </div>
                      <pre
                        style={{
                          background: "rgba(255,255,255,0.04)",
                          border: "1px solid var(--color-separator)",
                          borderRadius: "var(--radius-md)",
                          padding: 14,
                          fontSize: 12,
                          color: "var(--color-text-secondary)",
                          overflowX: "auto",
                          whiteSpace: "pre-wrap",
                        }}
                      >
                        {JSON.stringify(data, null, 2)}
                      </pre>
                    </div>
                  }
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  )
}
