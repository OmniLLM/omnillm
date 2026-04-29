import { useEffect, useMemo, useState, type CSSProperties } from "react"

import {
  getMeteringByModel,
  getMeteringByProvider,
  getMeteringLogs,
  getMeteringStats,
  type MeteringBreakdownItem,
  type MeteringLogsResponse,
  type MeteringQuery,
  type MeteringRecord,
  type MeteringStats,
} from "@/api"

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value)
}

function formatLatency(value: number) {
  return `${Math.round(value)} ms`
}

function formatDateTime(value: string) {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function isoInputValue(date: Date) {
  return date.toISOString().slice(0, 16)
}

function Card({
  children,
  style,
}: {
  children: React.ReactNode
  style?: CSSProperties
}) {
  return (
    <section
      style={{
        background: "var(--color-bg-elevated)",
        borderRadius: "var(--radius-lg)",
        border: "1px solid var(--color-separator)",
        boxShadow: "var(--shadow-card)",
        overflow: "hidden",
        ...style,
      }}
    >
      {children}
    </section>
  )
}

function StatCard({
  label,
  value,
  accent,
  subtext,
}: {
  label: string
  value: string
  accent: string
  subtext?: string
}) {
  return (
    <Card style={{ padding: 18, borderTop: `2px solid ${accent}` }}>
      <div
        style={{
          fontSize: 11,
          fontWeight: 700,
          letterSpacing: "0.08em",
          textTransform: "uppercase",
          color: "var(--color-text-tertiary)",
        }}
      >
        {label}
      </div>
      <div
        style={{
          marginTop: 10,
          fontSize: 28,
          fontWeight: 700,
          color: accent,
          fontFamily: "var(--font-mono)",
          letterSpacing: "-0.03em",
        }}
      >
        {value}
      </div>
      {subtext && (
        <div
          style={{
            marginTop: 8,
            fontSize: 12,
            color: "var(--color-text-secondary)",
          }}
        >
          {subtext}
        </div>
      )}
    </Card>
  )
}

function BreakdownTable({
  title,
  keyLabel,
  items,
  valueKey,
}: {
  title: string
  keyLabel: "model" | "provider"
  items: Array<MeteringBreakdownItem>
  valueKey: "model_id" | "provider_id"
}) {
  return (
    <Card>
      <div
        style={{
          padding: "14px 16px",
          borderBottom: "1px solid var(--color-separator)",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
        }}
      >
        <div>
          <h2
            style={{
              margin: 0,
              fontSize: 15,
              fontWeight: 700,
              color: "var(--color-text)",
            }}
          >
            {title}
          </h2>
        </div>
        <span
          style={{
            fontSize: 11,
            color: "var(--color-text-tertiary)",
            fontFamily: "var(--font-mono)",
          }}
        >
          {items.length} rows
        </span>
      </div>
      <div style={{ overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ background: "rgba(255,255,255,0.02)" }}>
              {[
                keyLabel,
                "requests",
                "input",
                "output",
                "total",
                "avg latency",
              ].map((label) => (
                <th
                  key={label}
                  style={{
                    textAlign: "left",
                    padding: "10px 14px",
                    fontSize: 11,
                    textTransform: "uppercase",
                    letterSpacing: "0.06em",
                    color: "var(--color-text-tertiary)",
                    borderBottom: "1px solid var(--color-separator)",
                  }}
                >
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.length === 0 && (
              <tr>
                <td
                  colSpan={6}
                  style={{
                    padding: "24px 14px",
                    color: "var(--color-text-secondary)",
                    textAlign: "center",
                  }}
                >
                  No data yet.
                </td>
              </tr>
            )}
            {items.map((item, index) => (
              <tr key={`${item[valueKey] ?? "unknown"}-${index}`}>
                <td
                  style={{
                    padding: "12px 14px",
                    borderBottom: "1px solid var(--color-separator)",
                    color: "var(--color-text)",
                    fontWeight: 600,
                  }}
                >
                  {item[valueKey] ?? "—"}
                </td>
                <td style={cellStyle}>{formatNumber(item.requests)}</td>
                <td style={cellStyle}>{formatNumber(item.input_tokens)}</td>
                <td style={cellStyle}>{formatNumber(item.output_tokens)}</td>
                <td style={cellStyle}>{formatNumber(item.total_tokens)}</td>
                <td style={cellStyle}>{formatLatency(item.avg_latency_ms)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  )
}

const cellStyle: CSSProperties = {
  padding: "12px 14px",
  borderBottom: "1px solid var(--color-separator)",
  color: "var(--color-text-secondary)",
  fontFamily: "var(--font-mono)",
  fontSize: 12,
}

export function MeteringPage({
  showToast,
}: {
  showToast: (msg: string, type?: "success" | "error") => void
}) {
  const now = useMemo(() => new Date(), [])
  const defaultSince = useMemo(() => {
    const value = new Date(now)
    value.setHours(value.getHours() - 24)
    return isoInputValue(value)
  }, [now])
  const defaultUntil = useMemo(() => isoInputValue(now), [now])

  const [since, setSince] = useState(defaultSince)
  const [until, setUntil] = useState(defaultUntil)
  const [modelId, setModelId] = useState("")
  const [providerId, setProviderId] = useState("")
  const [apiShape, setAPIShape] = useState("")
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(true)
  const [stats, setStats] = useState<MeteringStats | null>(null)
  const [logs, setLogs] = useState<MeteringLogsResponse | null>(null)
  const [byModel, setByModel] = useState<Array<MeteringBreakdownItem>>([])
  const [byProvider, setByProvider] = useState<Array<MeteringBreakdownItem>>([])

  const query = useMemo<MeteringQuery>(
    () => ({
      since: since ? new Date(since).toISOString() : undefined,
      until: until ? new Date(until).toISOString() : undefined,
      model_id: modelId || undefined,
      provider_id: providerId || undefined,
      api_shape: apiShape || undefined,
      page,
      page_size: 50,
    }),
    [apiShape, modelId, page, providerId, since, until],
  )

  useEffect(() => {
    let cancelled = false
    setLoading(true)

    Promise.all([
      getMeteringStats(query),
      getMeteringLogs(query),
      getMeteringByModel(query),
      getMeteringByProvider(query),
    ])
      .then(([nextStats, nextLogs, nextByModel, nextByProvider]) => {
        if (cancelled) return
        setStats(nextStats)
        setLogs(nextLogs)
        setByModel(nextByModel.items)
        setByProvider(nextByProvider.items)
      })
      .catch((e: unknown) => {
        if (cancelled) return
        showToast(
          "Failed to load metering data: "
            + (e instanceof Error ? e.message : String(e)),
          "error",
        )
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [query, showToast])

  const totalPages =
    logs ? Math.max(1, Math.ceil(logs.total / logs.page_size)) : 1

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "flex-start",
          gap: 16,
          flexWrap: "wrap",
        }}
      >
        <div>
          <h1
            style={{
              margin: 0,
              fontFamily: "var(--font-display)",
              fontWeight: 700,
              fontSize: 28,
              letterSpacing: "-0.02em",
              color: "var(--color-text)",
            }}
          >
            Metering
          </h1>
          <p
            style={{
              margin: "8px 0 0",
              fontSize: 14,
              color: "var(--color-text-secondary)",
              maxWidth: 720,
            }}
          >
            Request-level usage data captured after CIF responses finalize, with
            token totals, latency, provider attribution, and raw request logs.
          </p>
        </div>
        <button
          className="btn btn-ghost btn-sm"
          onClick={() => setPage(1)}
          disabled={loading}
        >
          Refresh
        </button>
      </div>

      <Card style={{ padding: 18 }}>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))",
            gap: 12,
          }}
        >
          <label style={fieldStyle}>
            <span style={labelStyle}>Since</span>
            <input
              className="sys-input"
              type="datetime-local"
              value={since}
              onChange={(e) => {
                setSince(e.target.value)
                setPage(1)
              }}
            />
          </label>
          <label style={fieldStyle}>
            <span style={labelStyle}>Until</span>
            <input
              className="sys-input"
              type="datetime-local"
              value={until}
              onChange={(e) => {
                setUntil(e.target.value)
                setPage(1)
              }}
            />
          </label>
          <label style={fieldStyle}>
            <span style={labelStyle}>Model</span>
            <input
              className="sys-input"
              value={modelId}
              onChange={(e) => {
                setModelId(e.target.value)
                setPage(1)
              }}
              placeholder="claude-3-7-sonnet"
            />
          </label>
          <label style={fieldStyle}>
            <span style={labelStyle}>Provider</span>
            <input
              className="sys-input"
              value={providerId}
              onChange={(e) => {
                setProviderId(e.target.value)
                setPage(1)
              }}
              placeholder="provider instance id"
            />
          </label>
          <label style={fieldStyle}>
            <span style={labelStyle}>API shape</span>
            <select
              className="sys-select"
              value={apiShape}
              onChange={(e) => {
                setAPIShape(e.target.value)
                setPage(1)
              }}
            >
              <option value="">All</option>
              <option value="openai">openai</option>
              <option value="anthropic">anthropic</option>
            </select>
          </label>
        </div>
      </Card>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(210px, 1fr))",
          gap: 14,
        }}
      >
        <StatCard
          label="Requests"
          value={formatNumber(stats?.total_requests ?? 0)}
          accent="var(--color-blue)"
          subtext="Completed requests in the selected window"
        />
        <StatCard
          label="Total tokens"
          value={formatNumber(stats?.total_tokens ?? 0)}
          accent="var(--color-green)"
          subtext={`${formatNumber(stats?.total_input_tokens ?? 0)} in / ${formatNumber(stats?.total_output_tokens ?? 0)} out`}
        />
        <StatCard
          label="Avg latency"
          value={formatLatency(stats?.avg_latency_ms ?? 0)}
          accent="var(--color-orange)"
          subtext="Wall-clock latency across successful and failed requests"
        />
        <StatCard
          label="Errors"
          value={formatNumber(stats?.error_count ?? 0)}
          accent="var(--color-red)"
          subtext="Requests recorded with a status code ≥ 400"
        />
      </div>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))",
          gap: 16,
        }}
      >
        <BreakdownTable
          title="By model"
          keyLabel="model"
          items={byModel}
          valueKey="model_id"
        />
        <BreakdownTable
          title="By provider"
          keyLabel="provider"
          items={byProvider}
          valueKey="provider_id"
        />
      </div>

      <Card>
        <div
          style={{
            padding: "14px 16px",
            borderBottom: "1px solid var(--color-separator)",
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            gap: 12,
            flexWrap: "wrap",
          }}
        >
          <div>
            <h2
              style={{
                margin: 0,
                fontSize: 15,
                fontWeight: 700,
                color: "var(--color-text)",
              }}
            >
              Request log
            </h2>
            <p
              style={{
                margin: "6px 0 0",
                fontSize: 12,
                color: "var(--color-text-secondary)",
              }}
            >
              Raw metering rows captured from OpenAI and Anthropic response
              paths.
            </p>
          </div>
          <div
            style={{
              fontSize: 11,
              color: "var(--color-text-tertiary)",
              fontFamily: "var(--font-mono)",
            }}
          >
            {loading ? "Loading…" : `${logs?.total ?? 0} rows`}
          </div>
        </div>
        <div style={{ overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ background: "rgba(255,255,255,0.02)" }}>
                {[
                  "time",
                  "model",
                  "provider",
                  "shape",
                  "input",
                  "output",
                  "total",
                  "latency",
                  "status",
                  "stream",
                ].map((label) => (
                  <th
                    key={label}
                    style={{
                      textAlign: "left",
                      padding: "10px 14px",
                      fontSize: 11,
                      textTransform: "uppercase",
                      letterSpacing: "0.06em",
                      color: "var(--color-text-tertiary)",
                      borderBottom: "1px solid var(--color-separator)",
                    }}
                  >
                    {label}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {!loading && (logs?.items.length ?? 0) === 0 && (
                <tr>
                  <td
                    colSpan={10}
                    style={{
                      padding: "28px 16px",
                      textAlign: "center",
                      color: "var(--color-text-secondary)",
                    }}
                  >
                    No metering records yet.
                  </td>
                </tr>
              )}
              {logs?.items.map((row: MeteringRecord) => (
                <tr key={row.id}>
                  <td style={cellStyle}>{formatDateTime(row.created_at)}</td>
                  <td style={cellStyle}>{row.model_id}</td>
                  <td style={cellStyle}>{row.provider_id}</td>
                  <td style={cellStyle}>{row.api_shape}</td>
                  <td style={cellStyle}>{formatNumber(row.input_tokens)}</td>
                  <td style={cellStyle}>{formatNumber(row.output_tokens)}</td>
                  <td style={cellStyle}>{formatNumber(row.total_tokens)}</td>
                  <td style={cellStyle}>{formatLatency(row.latency_ms)}</td>
                  <td
                    style={{
                      ...cellStyle,
                      color:
                        row.status_code >= 400 ?
                          "var(--color-red)"
                        : "var(--color-green)",
                    }}
                  >
                    {row.status_code}
                  </td>
                  <td style={cellStyle}>{row.is_stream ? "yes" : "no"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div
          style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            gap: 12,
            padding: "12px 16px",
            borderTop: "1px solid var(--color-separator)",
            flexWrap: "wrap",
          }}
        >
          <div
            style={{
              fontSize: 12,
              color: "var(--color-text-secondary)",
            }}
          >
            Page {logs?.page ?? page} of {totalPages}
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <button
              className="btn btn-ghost btn-sm"
              disabled={page <= 1 || loading}
              onClick={() => setPage((value) => Math.max(1, value - 1))}
            >
              Previous
            </button>
            <button
              className="btn btn-ghost btn-sm"
              disabled={page >= totalPages || loading}
              onClick={() => setPage((value) => value + 1)}
            >
              Next
            </button>
          </div>
        </div>
      </Card>
    </div>
  )
}

const fieldStyle: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 6,
}

const labelStyle: CSSProperties = {
  fontSize: 11,
  fontWeight: 700,
  textTransform: "uppercase",
  letterSpacing: "0.06em",
  color: "var(--color-text-tertiary)",
}
