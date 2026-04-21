import { useEffect, useState, type CSSProperties } from "react"

import {
  getInfo,
  getStatus,
  listProviders,
  type Provider,
  type ServerInfo,
  type Status,
} from "@/api"

function Spin() {
  return (
    <svg
      width="14"
      height="14"
      viewBox="0 0 16 16"
      className="animate-spin"
      style={{ flexShrink: 0 }}
    >
      <circle
        cx="8"
        cy="8"
        r="6"
        stroke="currentColor"
        strokeWidth="2"
        strokeDasharray="28"
        strokeDashoffset="10"
        fill="none"
        opacity="0.6"
      />
    </svg>
  )
}

function DataRow({
  label,
  value,
  accent,
}: {
  label: string
  value: string
  accent?: boolean
}) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        padding: "11px 16px",
        borderBottom: "1px solid var(--color-separator)",
      }}
    >
      <span style={{ fontSize: 13, color: "var(--color-text-secondary)" }}>
        {label}
      </span>
      <span
        style={{
          fontSize: 13,
          fontFamily: accent ? "var(--font-mono)" : "var(--font-text)",
          fontWeight: 500,
          color: accent ? "var(--color-blue)" : "var(--color-text)",
        }}
      >
        {value}
      </span>
    </div>
  )
}

const methodColor = (method: string) => {
  if (method === "GET") return "var(--color-green)"
  if (method === "POST") return "var(--color-blue)"
  return "var(--color-orange)"
}

export function AboutPage({
  showToast,
}: {
  showToast: (msg: string, type?: "success" | "error") => void
}) {
  const [status, setStatus] = useState<Status | null>(null)
  const [info, setInfo] = useState<ServerInfo | null>(null)
  const [providers, setProviders] = useState<Array<Provider>>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    Promise.all([getStatus(), getInfo(), listProviders()])
      .then(([nextStatus, nextInfo, nextProviders]) => {
        setStatus(nextStatus)
        setInfo(nextInfo)
        setProviders(nextProviders)
      })
      .catch((e: unknown) =>
        showToast(
          "Failed: " + (e instanceof Error ? e.message : String(e)),
          "error",
        ),
      )
      .finally(() => setLoading(false))
  }, [showToast])

  if (loading) {
    return (
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          gap: 10,
          padding: "80px 0",
          color: "var(--color-text-secondary)",
          fontSize: 14,
        }}
      >
        <Spin /> Loading…
      </div>
    )
  }

  if (!status) return null

  const port = info?.port ?? 4141
  const baseUrl = `http://localhost:${port}`
  const displayedProvider =
    status.activeProvider?.name
    ?? providers.find((provider) => provider.isActive)?.name
    ?? "None"

  const endpoints = [
    { method: "GET", path: "/v1/models" },
    { method: "POST", path: "/v1/chat/completions" },
    { method: "POST", path: "/v1/messages" },
    { method: "POST", path: "/v1/responses" },
    { method: "POST", path: "/v1/embeddings" },
    { method: "GET", path: "/usage" },
    { method: "GET", path: "/api/admin/providers" },
    { method: "POST", path: "/api/admin/providers/switch" },
    { method: "GET", path: "/api/admin/status" },
    { method: "GET", path: "/api/admin/settings/log-level" },
    { method: "PUT", path: "/api/admin/settings/log-level" },
    { method: "GET", path: "/api/admin/logs/stream" },
  ]

  const card: CSSProperties = {
    background: "var(--color-bg-elevated)",
    borderRadius: "var(--radius-lg)",
    border: "1px solid var(--color-separator)",
    boxShadow: "var(--shadow-card)",
    overflow: "hidden",
  }

  const copy = (value: string) => {
    void navigator.clipboard.writeText(value)
    showToast("Copied!")
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
      <section
        style={{
          ...card,
          padding: 24,
          display: "flex",
          flexDirection: "column",
          gap: 16,
        }}
      >
        <div>
          <h1
            style={{
              fontFamily: "var(--font-display)",
              fontWeight: 700,
              fontSize: 28,
              color: "var(--color-text)",
              letterSpacing: "-0.02em",
              lineHeight: 1,
              margin: 0,
            }}
          >
            About
          </h1>
          <p
            style={{
              fontSize: 14,
              color: "var(--color-text-secondary)",
              margin: "8px 0 0",
            }}
          >
            Connection details, runtime status, and quick integration endpoints.
          </p>
        </div>

        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
            gap: 12,
          }}
        >
          <div
            style={{
              ...card,
              padding: "16px 18px",
              borderTop: "2px solid var(--color-blue)",
            }}
          >
            <div
              style={{
                fontSize: 11,
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.06em",
                color: "var(--color-text-tertiary)",
              }}
            >
              Version
            </div>
            <div
              style={{
                marginTop: 8,
                fontSize: 20,
                fontWeight: 700,
                color: "var(--color-blue)",
                fontFamily: "var(--font-mono)",
              }}
            >
              {info?.version ?? "—"}
            </div>
          </div>
          <div
            style={{
              ...card,
              padding: "16px 18px",
              borderTop: "2px solid var(--color-green)",
            }}
          >
            <div
              style={{
                fontSize: 11,
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.06em",
                color: "var(--color-text-tertiary)",
              }}
            >
              Active Provider
            </div>
            <div
              style={{
                marginTop: 8,
                fontSize: 20,
                fontWeight: 700,
                color: "var(--color-text)",
              }}
            >
              {displayedProvider}
            </div>
          </div>
          <div
            style={{
              ...card,
              padding: "16px 18px",
              borderTop: "2px solid var(--color-purple)",
            }}
          >
            <div
              style={{
                fontSize: 11,
                fontWeight: 600,
                textTransform: "uppercase",
                letterSpacing: "0.06em",
                color: "var(--color-text-tertiary)",
              }}
            >
              Available Models
            </div>
            <div
              style={{
                marginTop: 8,
                fontSize: 20,
                fontWeight: 700,
                color: "var(--color-text)",
              }}
            >
              {status.modelCount}
            </div>
          </div>
        </div>
      </section>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1.1fr 0.9fr",
          gap: 24,
          alignItems: "start",
        }}
      >
        <section style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          <div>
            <div
              style={{
                fontSize: 12,
                fontWeight: 600,
                color: "var(--color-text-secondary)",
                marginBottom: 10,
                letterSpacing: "-0.01em",
              }}
            >
              Server Details
            </div>
            <div style={card}>
              <DataRow label="Version" value={info?.version ?? "—"} accent />
              <DataRow
                label="Port"
                value={info?.port ? `:${info.port}` : "—"}
                accent
              />
              <div style={{ padding: "11px 16px" }}>
                <div
                  style={{
                    fontSize: 12,
                    color: "var(--color-text-secondary)",
                    marginBottom: 6,
                  }}
                >
                  Base URL
                </div>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <code
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 12,
                      color: "var(--color-blue)",
                      flex: 1,
                      background: "rgba(10,132,255,0.08)",
                      borderRadius: "var(--radius-sm)",
                      padding: "5px 10px",
                    }}
                  >
                    {baseUrl}
                  </code>
                  <button
                    className="btn btn-ghost btn-sm"
                    onClick={() => copy(baseUrl)}
                  >
                    Copy
                  </button>
                </div>
              </div>
            </div>
          </div>

          <div>
            <div
              style={{
                fontSize: 12,
                fontWeight: 600,
                color: "var(--color-text-secondary)",
                marginBottom: 10,
              }}
            >
              API Endpoints
            </div>
            <div style={card}>
              {endpoints.map((endpoint, index) => (
                <div
                  key={`${endpoint.method}-${endpoint.path}`}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 12,
                    padding: "10px 16px",
                    borderBottom:
                      index < endpoints.length - 1 ?
                        "1px solid var(--color-separator)"
                      : "none",
                  }}
                >
                  <span
                    style={{
                      fontSize: 10,
                      fontWeight: 700,
                      fontFamily: "var(--font-mono)",
                      color: methodColor(endpoint.method),
                      minWidth: 38,
                      letterSpacing: "0.03em",
                    }}
                  >
                    {endpoint.method}
                  </span>
                  <span
                    style={{
                      fontFamily: "var(--font-mono)",
                      fontSize: 12,
                      color: "var(--color-text-secondary)",
                    }}
                  >
                    {endpoint.path}
                  </span>
                </div>
              ))}
            </div>
          </div>
        </section>

        <section style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          <div>
            <div
              style={{
                fontSize: 12,
                fontWeight: 600,
                color: "var(--color-text-secondary)",
                marginBottom: 10,
              }}
            >
              Runtime State
            </div>
            <div style={card}>
              <DataRow label="Active Provider" value={displayedProvider} />
              <DataRow
                label="Available Models"
                value={String(status.modelCount)}
              />
              <DataRow
                label="Manual Approve"
                value={status.manualApprove ? "Enabled" : "Disabled"}
              />
              <DataRow
                label="Rate Limit"
                value={
                  status.rateLimitSeconds !== null ?
                    `${status.rateLimitSeconds}s`
                  : "None"
                }
              />
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  padding: "11px 16px",
                }}
              >
                <span
                  style={{ fontSize: 13, color: "var(--color-text-secondary)" }}
                >
                  Rate Limit Mode
                </span>
                <span
                  style={{
                    fontSize: 12,
                    fontWeight: 600,
                    padding: "3px 10px",
                    borderRadius: "var(--radius-pill)",
                    background:
                      status.rateLimitWait ?
                        "var(--color-green-fill)"
                      : "var(--color-orange-fill)",
                    color:
                      status.rateLimitWait ? "var(--color-green)" : (
                        "var(--color-orange)"
                      ),
                  }}
                >
                  {status.rateLimitWait ? "Wait" : "Error"}
                </span>
              </div>
            </div>
          </div>

          <div>
            <div
              style={{
                fontSize: 12,
                fontWeight: 600,
                color: "var(--color-text-secondary)",
                marginBottom: 10,
              }}
            >
              Quick Copy
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
              {[
                {
                  label: "OpenAI Base URL",
                  value: `${baseUrl}/v1`,
                },
                { label: "Claude API Base", value: baseUrl },
              ].map(({ label, value }) => (
                <div key={label} style={{ ...card, padding: "12px 16px" }}>
                  <div
                    style={{
                      fontSize: 12,
                      color: "var(--color-text-secondary)",
                      marginBottom: 6,
                    }}
                  >
                    {label}
                  </div>
                  <div
                    style={{ display: "flex", alignItems: "center", gap: 10 }}
                  >
                    <code
                      style={{
                        fontFamily: "var(--font-mono)",
                        fontSize: 12,
                        color: "var(--color-blue)",
                        flex: 1,
                        background: "rgba(10,132,255,0.08)",
                        borderRadius: "var(--radius-sm)",
                        padding: "5px 10px",
                      }}
                    >
                      {value}
                    </code>
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={() => copy(value)}
                    >
                      Copy
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>
      </div>
    </div>
  )
}
