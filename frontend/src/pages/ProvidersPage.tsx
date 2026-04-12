import { useCallback, useEffect, useRef, useState } from "react"

import {
  activateProvider,
  addProviderInstance,
  authProvider,
  cancelAuth,
  deactivateProvider,
  deleteProvider,
  getAuthStatus,
  getProviderModels,
  getProviderUsage,
  listProviders,
  getStatus,
  toggleProviderModel,
  getProviderPriorities,
  setProviderPriorities,
  updateProviderConfig,
  type AuthFlow,
  type Model,
  type Provider,
  type Status,
  type UsageData,
} from "@/api"

import { createLogger } from "@/lib/logger"

const _log = createLogger("providers-page")

interface ProvidersPageProps {
  showToast: (msg: string, type?: "success" | "error") => void
}

// ─── Spinner ──────────────────────────────────────────────────────────────────

function Spin({ size = 14 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
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

// ─── Auth Flow Banner ─────────────────────────────────────────────────────────

function AuthFlowBanner({
  authFlow,
  providers,
  onCancel,
}: {
  authFlow: AuthFlow | null | undefined
  providers: Array<Provider>
  onCancel: () => void
}) {
  if (
    !authFlow
    || authFlow.status === "complete"
    || authFlow.status === "error"
  )
    return null
  const name =
    providers.find((p) => p.id === authFlow.providerId)?.name
    ?? authFlow.providerId

  return (
    <div
      style={{
        background: "rgba(255,159,10,0.1)",
        border: "1px solid rgba(255,159,10,0.25)",
        borderRadius: "var(--radius-lg)",
        padding: "14px 18px",
        marginBottom: 24,
      }}
    >
      {authFlow.status === "pending" && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            color: "var(--color-orange)",
            fontSize: 14,
            fontWeight: 500,
          }}
        >
          <Spin size={14} />
          Initiating auth flow for {name}…
        </div>
      )}
      {authFlow.status === "awaiting_user" && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div
            style={{
              color: "var(--color-orange)",
              fontSize: 14,
              fontWeight: 600,
            }}
          >
            Authorization Required — {name}
          </div>
          {authFlow.userCode && (
            <div>
              <div
                style={{
                  fontSize: 12,
                  color: "var(--color-text-secondary)",
                  marginBottom: 6,
                }}
              >
                Enter this code:
              </div>
              <div
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 22,
                  fontWeight: 700,
                  color: "var(--color-orange)",
                  letterSpacing: "0.2em",
                  background: "rgba(255,159,10,0.08)",
                  border: "1px solid rgba(255,159,10,0.2)",
                  borderRadius: "var(--radius-md)",
                  padding: "10px 16px",
                  display: "inline-block",
                }}
              >
                {authFlow.userCode}
              </div>
            </div>
          )}
          {authFlow.instructionURL && (
            <div>
              <div
                style={{
                  fontSize: 12,
                  color: "var(--color-text-secondary)",
                  marginBottom: 6,
                }}
              >
                Authorization URL:
              </div>
              <div
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 11,
                  color: "var(--color-text-secondary)",
                  background: "rgba(255,255,255,0.04)",
                  border: "1px solid var(--color-separator)",
                  borderRadius: "var(--radius-sm)",
                  padding: "8px 12px",
                  wordBreak: "break-all",
                  marginBottom: 10,
                }}
              >
                {authFlow.instructionURL}
              </div>
              <a
                href={authFlow.instructionURL}
                target="_blank"
                rel="noopener noreferrer"
              >
                <button className="btn btn-amber btn-sm">
                  Open in Browser ↗
                </button>
              </a>
            </div>
          )}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              color: "var(--color-text-secondary)",
              fontSize: 13,
            }}
          >
            <Spin size={13} />
            Waiting for authorization…
          </div>
          <div>
            <button
              className="btn btn-sm"
              style={{ color: "var(--color-text-secondary)" }}
              onClick={onCancel}
            >
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Inline Auth Forms ────────────────────────────────────────────────────────

function FormRow({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div>
      <label className="sys-label">{label}</label>
      {children}
    </div>
  )
}

function AuthFormWrapper({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <div
      style={{
        marginTop: 16,
        background: "rgba(10,132,255,0.06)",
        border: "1px solid rgba(10,132,255,0.18)",
        borderRadius: "var(--radius-lg)",
        padding: 16,
        display: "flex",
        flexDirection: "column",
        gap: 12,
        animation: "slide-up 0.18s var(--ease) both",
      }}
    >
      <div
        style={{ fontSize: 13, fontWeight: 600, color: "var(--color-blue)" }}
      >
        {title}
      </div>
      {children}
    </div>
  )
}

function AlibabaAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [method, setMethod] = useState("api-key")
  const [region, setRegion] = useState("global")
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    const body: Record<string, string> = { method }
    if (method === "api-key") {
      if (!apiKey.trim()) return
      body.apiKey = apiKey.trim()
      body.region = region
    }
    await onSubmit(body)
  }
  return (
    <AuthFormWrapper title="Authenticate Alibaba DashScope">
      <FormRow label="Auth Method">
        <select
          className="sys-select"
          value={method}
          onChange={(e) => setMethod(e.target.value)}
        >
          <option value="api-key">API Key</option>
          <option value="oauth">OAuth (device flow)</option>
        </select>
      </FormRow>
      {method === "api-key" && (
        <>
          <FormRow label="Region">
            <select
              className="sys-select"
              value={region}
              onChange={(e) => setRegion(e.target.value)}
            >
              <option value="global">
                Global (dashscope-intl.aliyuncs.com)
              </option>
              <option value="china">China (dashscope.aliyuncs.com)</option>
            </select>
          </FormRow>
          <FormRow label="DashScope API Key">
            <input
              className="sys-input"
              type="password"
              placeholder="sk-…"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
            />
          </FormRow>
        </>
      )}
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Submit
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function CopilotAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [method, setMethod] = useState("oauth")
  const [token, setToken] = useState("")
  const submit = async () => {
    const body: Record<string, string> = { method }
    if (method === "token") {
      if (!token.trim()) return
      body.token = token.trim()
    }
    await onSubmit(body)
  }
  return (
    <AuthFormWrapper title="Authenticate GitHub Copilot">
      <FormRow label="Auth Method">
        <select
          className="sys-select"
          value={method}
          onChange={(e) => setMethod(e.target.value)}
        >
          <option value="oauth">OAuth device flow (browser)</option>
          <option value="token">Paste existing token</option>
        </select>
      </FormRow>
      {method === "token" && (
        <FormRow label="GitHub Token">
          <input
            className="sys-input"
            type="password"
            placeholder="ghu_…"
            value={token}
            onChange={(e) => setToken(e.target.value)}
          />
        </FormRow>
      )}
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Submit
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function AntigravityAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [clientId, setClientId] = useState("")
  const [clientSecret, setClientSecret] = useState("")
  const submit = async () => {
    if (!clientId.trim() || !clientSecret.trim()) return
    await onSubmit({
      method: "oauth",
      clientId: clientId.trim(),
      clientSecret: clientSecret.trim(),
    })
  }
  return (
    <AuthFormWrapper title="Authenticate Antigravity (Google)">
      <FormRow label="OAuth Client ID">
        <input
          className="sys-input"
          type="text"
          placeholder="…apps.googleusercontent.com"
          value={clientId}
          onChange={(e) => setClientId(e.target.value)}
        />
      </FormRow>
      <FormRow label="OAuth Client Secret">
        <input
          className="sys-input"
          type="password"
          placeholder="GOCSPX-…"
          value={clientSecret}
          onChange={(e) => setClientSecret(e.target.value)}
        />
      </FormRow>
      <div style={{ fontSize: 12, color: "var(--color-text-tertiary)" }}>
        Opens a Google OAuth browser flow once submitted.
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Start OAuth
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

function AzureOpenAIAuthForm({
  onSubmit,
  onCancel,
}: {
  onSubmit: (body: Record<string, string>) => Promise<void>
  onCancel: () => void
}) {
  const [apiKey, setApiKey] = useState("")
  const submit = async () => {
    if (!apiKey.trim()) return
    await onSubmit({
      apiKey: apiKey.trim(),
    })
  }
  return (
    <AuthFormWrapper title="Authenticate Azure OpenAI">
      <FormRow label="API Key">
        <input
          className="sys-input"
          type="password"
          placeholder="Enter your Azure OpenAI API key"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
        />
      </FormRow>
      <div style={{ fontSize: 12, color: "var(--color-text-tertiary)" }}>
        Enter your Azure OpenAI API key. Use the "Configure" button to set
        endpoint and deployments.
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <button className="btn btn-primary btn-sm" onClick={submit}>
          Submit
        </button>
        <button className="btn btn-ghost btn-sm" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </AuthFormWrapper>
  )
}

// ─── Models Dialog ────────────────────────────────────────────────────────────

function ModelsDialog({
  provider,
  onModelsChanged,
}: {
  provider: Provider
  onModelsChanged?: () => void
}) {
  const [models, setModels] = useState<Array<Model> | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState("")
  const [configLoading, setConfigLoading] = useState(false)

  // Azure OpenAI deployment management
  const [newDeployment, setNewDeployment] = useState("")
  const [deployments, setDeployments] = useState<Array<string>>(
    provider.config?.deployments || [],
  )

  const load = async () => {
    setLoading(true)
    setError(null)
    try {
      setModels((await getProviderModels(provider.id)).models)
      // Also update deployments from current config
      if (provider.type === "azure-openai") {
        setDeployments(provider.config?.deployments || [])
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleOpen = () => {
    setOpen(true)
    if (models === null && !loading) load()
    setNewDeployment("")
  }

  const handleToggle = async (model: Model) => {
    const newEnabled = !model.enabled
    setModels((prev) =>
      prev ?
        prev.map((m) => (m.id === model.id ? { ...m, enabled: newEnabled } : m))
      : prev,
    )
    try {
      await toggleProviderModel(provider.id, model.id, newEnabled)
      onModelsChanged?.()
    } catch {
      setModels((prev) =>
        prev ?
          prev.map((m) =>
            m.id === model.id ? { ...m, enabled: model.enabled } : m,
          )
        : prev,
      )
    }
  }

  const handleAddDeployment = async () => {
    if (!newDeployment.trim() || provider.type !== "azure-openai") return

    const deploymentName = newDeployment.trim()
    if (deployments.includes(deploymentName)) {
      setError("Deployment already exists")
      return
    }

    setConfigLoading(true)
    setError(null)
    try {
      const newDeployments = [...deployments, deploymentName]
      await updateProviderConfig(provider.id, {
        endpoint: provider.config?.endpoint,
        apiVersion: provider.config?.apiVersion || "2024-02-01",
        deployments: newDeployments,
      })
      setDeployments(newDeployments)
      setNewDeployment("")
      onModelsChanged?.() // Refresh provider data
      // Reload models to show the new deployment
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleRemoveDeployment = async (deploymentName: string) => {
    if (provider.type !== "azure-openai") return

    setConfigLoading(true)
    setError(null)
    try {
      const newDeployments = deployments.filter((d) => d !== deploymentName)
      await updateProviderConfig(provider.id, {
        endpoint: provider.config?.endpoint,
        apiVersion: provider.config?.apiVersion || "2024-02-01",
        deployments: newDeployments,
      })
      setDeployments(newDeployments)
      onModelsChanged?.() // Refresh provider data
      // Reload models to reflect the removed deployment
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setConfigLoading(false)
    }
  }

  const handleSelectAll = async () => {
    if (!models) return
    const toToggle = models.filter((m) => !m.enabled)
    setModels((prev) =>
      prev ? prev.map((m) => ({ ...m, enabled: true })) : prev,
    )
    try {
      await Promise.all(
        toToggle.map((m) => toggleProviderModel(provider.id, m.id, true)),
      )
      onModelsChanged?.()
    } catch {
      await load()
    }
  }

  const handleDeselectAll = async () => {
    if (!models) return
    const toToggle = models.filter((m) => m.enabled)
    setModels((prev) =>
      prev ? prev.map((m) => ({ ...m, enabled: false })) : prev,
    )
    try {
      await Promise.all(
        toToggle.map((m) => toggleProviderModel(provider.id, m.id, false)),
      )
      onModelsChanged?.()
    } catch {
      await load()
    }
  }

  const filtered =
    models ?
      models.filter((m) => m.id.toLowerCase().includes(search.toLowerCase()))
    : []
  const enabledCount = models ? models.filter((m) => m.enabled).length : null

  return (
    <>
      <button className="btn btn-ghost btn-sm" onClick={handleOpen}>
        Models
        {provider.totalModelCount != null && provider.totalModelCount > 0 && (
          <span
            style={{
              color: "var(--color-blue)",
              fontSize: 11,
              fontWeight: 600,
            }}
          >
            {provider.enabledModelCount}/{provider.totalModelCount}
          </span>
        )}
      </button>

      {open && (
        <div
          className="dialog-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="dialog-box">
            <div className="dialog-header">
              <div>
                <div
                  style={{
                    fontFamily: "var(--font-display)",
                    fontWeight: 600,
                    fontSize: 15,
                    color: "var(--color-text)",
                  }}
                >
                  {provider.name} — Models
                </div>
                {enabledCount !== null && models && (
                  <div
                    style={{
                      fontSize: 12,
                      color: "var(--color-text-secondary)",
                      marginTop: 2,
                    }}
                  >
                    <span style={{ color: "var(--color-green)" }}>
                      {enabledCount}
                    </span>{" "}
                    of {models.length} enabled
                  </div>
                )}
              </div>
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => setOpen(false)}
              >
                Done
              </button>
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
                  <Spin /> Loading models…
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
              {models && !loading && (
                <div
                  style={{ display: "flex", flexDirection: "column", gap: 14 }}
                >
                  {/* Azure OpenAI deployment management */}
                  {provider.type === "azure-openai" && (
                    <div
                      style={{
                        padding: "14px 16px",
                        background: "rgba(10,132,255,0.06)",
                        border: "1px solid rgba(10,132,255,0.18)",
                        borderRadius: "var(--radius-lg)",
                      }}
                    >
                      <div
                        style={{
                          fontSize: 13,
                          fontWeight: 600,
                          color: "var(--color-blue)",
                          marginBottom: 12,
                        }}
                      >
                        Deployment Management
                      </div>
                      <div
                        style={{
                          display: "flex",
                          gap: 8,
                          alignItems: "center",
                          marginBottom: 8,
                        }}
                      >
                        <input
                          className="sys-input"
                          placeholder="Add deployment name..."
                          value={newDeployment}
                          onChange={(e) => setNewDeployment(e.target.value)}
                          disabled={configLoading}
                          style={{ flex: 1, fontSize: 13 }}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              e.preventDefault()
                              handleAddDeployment()
                            }
                          }}
                        />
                        <button
                          className="btn btn-primary btn-sm"
                          onClick={handleAddDeployment}
                          disabled={configLoading || !newDeployment.trim()}
                          style={{ minWidth: 32, padding: "6px 8px" }}
                        >
                          {configLoading ?
                            <Spin size={12} />
                          : "+"}
                        </button>
                      </div>
                      <div
                        style={{
                          fontSize: 11,
                          color: "var(--color-text-tertiary)",
                        }}
                      >
                        Enter deployment names from your Azure OpenAI resource.
                        Each deployment becomes a model.
                      </div>
                    </div>
                  )}

                  <div
                    style={{ display: "flex", gap: 8, alignItems: "center" }}
                  >
                    <input
                      className="sys-input"
                      placeholder="Filter models…"
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                      style={{ flex: 1 }}
                    />
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={handleSelectAll}
                    >
                      All On
                    </button>
                    <button
                      className="btn btn-ghost btn-sm"
                      onClick={handleDeselectAll}
                    >
                      All Off
                    </button>
                  </div>
                  <div
                    style={{
                      borderRadius: "var(--radius-lg)",
                      overflow: "hidden",
                      border: "1px solid var(--color-separator)",
                    }}
                  >
                    {filtered.map((m, i) => (
                      <div
                        key={m.id}
                        style={{
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "space-between",
                          padding: "10px 14px",
                          borderBottom:
                            i < filtered.length - 1 ?
                              "1px solid var(--color-separator)"
                            : "none",
                          background:
                            m.enabled ? "rgba(48,209,88,0.04)" : "transparent",
                          transition: "background 0.12s",
                        }}
                      >
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 12,
                            flex: 1,
                          }}
                        >
                          <div>
                            <div
                              style={{
                                fontFamily: "var(--font-mono)",
                                fontSize: 13,
                                color:
                                  m.enabled ? "var(--color-text)" : (
                                    "var(--color-text-secondary)"
                                  ),
                              }}
                            >
                              {m.id}
                            </div>
                            {m.name && m.name !== m.id && (
                              <div
                                style={{
                                  fontSize: 11,
                                  color: "var(--color-text-tertiary)",
                                  marginTop: 1,
                                }}
                              >
                                {m.name}
                              </div>
                            )}
                          </div>
                        </div>
                        <div
                          style={{
                            display: "flex",
                            alignItems: "center",
                            gap: 8,
                          }}
                        >
                          {/* Remove deployment button for Azure OpenAI */}
                          {provider.type === "azure-openai"
                            && deployments.includes(m.id) && (
                              <button
                                className="btn btn-ghost btn-sm"
                                onClick={() => handleRemoveDeployment(m.id)}
                                disabled={configLoading}
                                style={{
                                  minWidth: 32,
                                  padding: "4px 6px",
                                  color: "var(--color-red)",
                                }}
                                title={`Remove deployment ${m.id}`}
                              >
                                {configLoading ?
                                  <Spin size={10} />
                                : "−"}
                              </button>
                            )}
                          <div
                            className={`toggle-track ${m.enabled ? "on" : ""}`}
                            onClick={() => handleToggle(m)}
                          >
                            <div className="toggle-thumb" />
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </>
  )
}

// ─── Usage Dialog ─────────────────────────────────────────────────────────────

function UsageDialog({ provider }: { provider: Provider }) {
  const [data, setData] = useState<UsageData | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [open, setOpen] = useState(false)

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
          <div className="dialog-box">
            <div className="dialog-header">
              <div
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
                  <Spin /> Fetching usage data…
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
                          fontSize: 12,
                          fontFamily: "var(--font-mono)",
                          color: "var(--color-text-secondary)",
                          background: "rgba(255,255,255,0.04)",
                          border: "1px solid var(--color-separator)",
                          borderRadius: "var(--radius-md)",
                          padding: 14,
                          overflow: "auto",
                          maxHeight: 280,
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

// ─── Priority Modal ───────────────────────────────────────────────────────────

function PriorityModal({
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
      <button
        className="btn btn-ghost btn-sm"
        onClick={openModal}
      >
        Priority
      </button>
      {open && (
        <div
          className="dialog-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="dialog-box" style={{ maxWidth: 420 }}>
            <div className="dialog-header">
              <div
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
                    <div style={{ display: "flex", gap: 4, alignItems: "center" }}>
                      <button
                        onClick={() => handleMove(i, -1)}
                        disabled={i === 0}
                        style={{
                          background: "transparent",
                          border: "1px solid var(--color-separator)",
                          borderRadius: 4,
                          color: i === 0 ? "var(--color-text-tertiary)" : "var(--color-text)",
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
                          color: i === ordered.length - 1 ? "var(--color-text-tertiary)" : "var(--color-text)",
                          cursor: i === ordered.length - 1 ? "not-allowed" : "pointer",
                          opacity: i === ordered.length - 1 ? 0.3 : 1,
                          padding: "2px 8px",
                          fontSize: 14,
                          lineHeight: 1,
                          pointerEvents: i === ordered.length - 1 ? "none" : "auto",
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

// ─── Provider Card ────────────────────────────────────────────────────────────

const PROVIDER_ACCENT: Record<string, string> = {
  "github-copilot": "#0a84ff",
  antigravity: "#30d158",
  alibaba: "#ff9f0a",
  "azure-openai": "#0078d4",
}

const PROVIDER_ICONS: Record<string, React.ReactNode> = {
  "github-copilot": (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.009-.868-.013-1.703-2.782.604-3.369-1.342-3.369-1.342-.454-1.155-1.11-1.463-1.11-1.463-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.161 22 16.416 22 12c0-5.523-4.477-10-10-10z" />
    </svg>
  ),
  antigravity: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path
        d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
        opacity=".7"
      />
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
      <path
        d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
        opacity=".7"
      />
    </svg>
  ),
  alibaba: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
    </svg>
  ),
  "azure-openai": (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10c1.19 0 2.34-.21 3.41-.6.39-.14.65-.5.65-.93 0-.55-.45-1-1-1-.24 0-.46.08-.64.21-.82.3-1.7.45-2.59.45-3.86 0-7-3.14-7-7s3.14-7 7-7 7 3.14 7 7c0 .89-.15 1.77-.45 2.59-.13.18-.21.4-.21.64 0 .55.45 1 1 1 .43 0 .79-.26.93-.65.39-1.07.6-2.22.6-3.41C22 6.48 17.52 2 12 2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  ),
}

function ProviderCard({
  provider,
  isFlowRunning,
  isActivating,
  onActivate,
  onDeactivate,
  onDelete,
  onAuthSubmit,
  onModelsChanged,
  priorityIndex,
  multiProvider,
}: {
  provider: Provider
  isFlowRunning: boolean
  isActivating: boolean
  onActivate: (id: string) => void
  onDeactivate: (id: string) => void
  onDelete: (id: string) => void
  onAuthSubmit: (id: string, body: Record<string, string>) => Promise<void>
  onModelsChanged: () => void
  priorityIndex: number
  multiProvider: boolean
}) {
  const [showAuthForm, setShowAuthForm] = useState(false)
  const accent = PROVIDER_ACCENT[provider.type] ?? "#0a84ff"

  const handleAuthSubmit = async (body: Record<string, string>) => {
    await onAuthSubmit(provider.id, body)
    setShowAuthForm(false)
  }

  return (
    <div
      style={{
        background: "var(--color-bg-elevated)",
        borderRadius: "var(--radius-lg)",
        border: "1px solid",
        borderColor:
          provider.isActive ? "rgba(48,209,88,0.3)" : "var(--color-separator)",
        boxShadow:
          provider.isActive ?
            "var(--shadow-card), 0 0 0 1px rgba(48,209,88,0.15)"
          : "var(--shadow-card)",
        overflow: "hidden",
        transition: "all 0.2s var(--ease)",
      }}
    >
      {/* Colored left bar for active */}
      {provider.isActive && (
        <div
          style={{
            height: 3,
            background: `linear-gradient(to right, ${accent}, var(--color-green))`,
          }}
        />
      )}

      <div style={{ padding: "16px 18px" }}>
        {/* Header */}
        <div
          style={{
            display: "flex",
            alignItems: "flex-start",
            justifyContent: "space-between",
            marginBottom: 14,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            {/* Icon badge */}
            <div
              style={{
                width: 42,
                height: 42,
                borderRadius: "var(--radius-md)",
                background: `${accent}18`,
                border: `1px solid ${accent}28`,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                color: accent,
                flexShrink: 0,
              }}
            >
              {PROVIDER_ICONS[provider.type] ?? (
                <span style={{ fontSize: 20 }}>◌</span>
              )}
            </div>
            <div>
              <div
                style={{
                  fontFamily: "var(--font-display)",
                  fontWeight: 600,
                  fontSize: 15,
                  color: "var(--color-text)",
                  lineHeight: 1.2,
                  letterSpacing: "-0.01em",
                }}
              >
                {provider.name}
              </div>
              <div
                style={{
                  fontSize: 11,
                  color: "var(--color-text-tertiary)",
                  marginTop: 2,
                  fontFamily: "var(--font-mono)",
                }}
              >
                {provider.id}
              </div>
            </div>
          </div>

          {/* Right badges */}
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            {provider.isActive && multiProvider && priorityIndex >= 0 && (
              <div
                style={{
                  fontSize: 11,
                  fontWeight: 600,
                  padding: "3px 10px",
                  background: "var(--color-blue-fill)",
                  color: "var(--color-blue)",
                  borderRadius: "var(--radius-pill)",
                }}
              >
                #{priorityIndex + 1}
              </div>
            )}
            {provider.isActive ?
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div className="status-dot status-dot-active" />
                <span
                  style={{
                    fontSize: 12,
                    fontWeight: 500,
                    color: "var(--color-green)",
                  }}
                >
                  Active
                </span>
              </div>
            : <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <div className="status-dot status-dot-inactive" />
                <span
                  style={{ fontSize: 12, color: "var(--color-text-tertiary)" }}
                >
                  {provider.authStatus === "authenticated" ?
                    "Ready"
                  : "Not authorized"}
                </span>
              </div>
            }
          </div>
        </div>

        {/* Model progress */}
        {provider.authStatus === "authenticated"
          && provider.totalModelCount != null
          && provider.totalModelCount > 0 && (
            <div style={{ marginBottom: 14 }}>
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  marginBottom: 5,
                  fontSize: 12,
                  color: "var(--color-text-secondary)",
                }}
              >
                <span>Models enabled</span>
                <span
                  style={{ fontFamily: "var(--font-mono)", fontWeight: 500 }}
                >
                  <span style={{ color: "var(--color-green)" }}>
                    {provider.enabledModelCount}
                  </span>
                  <span style={{ color: "var(--color-text-tertiary)" }}>
                    {" "}
                    / {provider.totalModelCount}
                  </span>
                </span>
              </div>
              <div className="progress-track">
                <div
                  className="progress-bar"
                  style={{
                    width: `${((provider.enabledModelCount ?? 0) / provider.totalModelCount) * 100}%`,
                    background: "var(--color-green)",
                  }}
                />
              </div>
            </div>
          )}

        {/* Actions */}
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: 8,
            alignItems: "center",
          }}
        >
          {provider.isActive ?
            <button
              className="btn btn-ghost btn-sm"
              disabled={isFlowRunning || isActivating}
              onClick={() => onDeactivate(provider.id)}
            >
              {isActivating ?
                <>
                  <Spin size={12} /> Working…
                </>
              : "Deactivate"}
            </button>
          : <button
              className="btn btn-green btn-sm"
              disabled={
                isFlowRunning
                || isActivating
                || provider.authStatus !== "authenticated"
              }
              onClick={() => onActivate(provider.id)}
            >
              {isActivating ?
                <>
                  <Spin size={12} /> Working…
                </>
              : "Activate"}
            </button>
          }
          <button
            className="btn btn-ghost btn-sm"
            disabled={isFlowRunning}
            onClick={() => setShowAuthForm((v) => !v)}
          >
            {showAuthForm ? "Cancel" : "Authorize"}
          </button>
          <ModelsDialog provider={provider} onModelsChanged={onModelsChanged} />
          <UsageDialog provider={provider} />
          <div style={{ flex: 1 }} />
          <button
            className="btn btn-danger btn-sm"
            disabled={isFlowRunning}
            onClick={() => {
              if (confirm(`Delete "${provider.name}"?`)) onDelete(provider.id)
            }}
          >
            Delete
          </button>
        </div>
      </div>

      {/* Auth form inlined */}
      {showAuthForm && (
        <div
          style={{
            borderTop: "1px solid var(--color-separator)",
            padding: "0 18px 18px",
          }}
        >
          {provider.type === "alibaba" && (
            <AlibabaAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "github-copilot" && (
            <CopilotAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "azure-openai" && (
            <AzureOpenAIAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
          {provider.type === "antigravity" && (
            <AntigravityAuthForm
              onSubmit={handleAuthSubmit}
              onCancel={() => setShowAuthForm(false)}
            />
          )}
        </div>
      )}
    </div>
  )
}

// ─── Add Provider Modal ───────────────────────────────────────────────────────

const PROVIDER_TYPES = [
  {
    id: "github-copilot",
    name: "GitHub Copilot",
    desc: "Access Copilot models via OAuth or token",
  },
  {
    id: "antigravity",
    name: "Antigravity (Google)",
    desc: "Google Vertex AI via OAuth client credentials",
  },
  {
    id: "alibaba",
    name: "Alibaba DashScope",
    desc: "Qwen models via API key or OAuth",
  },
  {
    id: "azure-openai",
    name: "Azure OpenAI",
    desc: "Azure OpenAI Service with your own deployments",
  },
]

function AddProviderModal({
  onAdd,
  disabled,
}: {
  onAdd: (type: string) => void
  disabled: boolean
}) {
  const [open, setOpen] = useState(false)

  return (
    <>
      <button
        className="btn btn-primary btn-sm"
        disabled={disabled}
        onClick={() => setOpen(true)}
      >
        Add Provider
      </button>
      {open && (
        <div
          className="dialog-overlay"
          onClick={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="dialog-box" style={{ maxWidth: 460 }}>
            <div className="dialog-header">
              <div
                style={{
                  fontFamily: "var(--font-display)",
                  fontWeight: 600,
                  fontSize: 15,
                }}
              >
                Add Provider
              </div>
              <button
                className="btn btn-ghost btn-sm"
                onClick={() => setOpen(false)}
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
                Select a provider type. You can add multiple accounts of the
                same type.
              </p>
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {PROVIDER_TYPES.map((pt) => {
                  const accent = PROVIDER_ACCENT[pt.id] ?? "#0a84ff"
                  return (
                    <button
                      key={pt.id}
                      onClick={() => {
                        onAdd(pt.id)
                        setOpen(false)
                      }}
                      style={{
                        display: "flex",
                        alignItems: "center",
                        gap: 14,
                        padding: "14px 16px",
                        background: "rgba(255,255,255,0.04)",
                        border: "1px solid var(--color-separator)",
                        borderRadius: "var(--radius-md)",
                        cursor: "pointer",
                        textAlign: "left",
                        transition: "all 0.15s var(--ease)",
                        width: "100%",
                        color: "var(--color-text)",
                      }}
                      onMouseEnter={(e) => {
                        ;(
                          e.currentTarget as HTMLButtonElement
                        ).style.background = `${accent}10`
                        ;(
                          e.currentTarget as HTMLButtonElement
                        ).style.borderColor = `${accent}30`
                      }}
                      onMouseLeave={(e) => {
                        ;(
                          e.currentTarget as HTMLButtonElement
                        ).style.background = "rgba(255,255,255,0.04)"
                        ;(
                          e.currentTarget as HTMLButtonElement
                        ).style.borderColor = "var(--color-separator)"
                      }}
                    >
                      <div
                        style={{
                          width: 38,
                          height: 38,
                          borderRadius: "var(--radius-sm)",
                          background: `${accent}18`,
                          border: `1px solid ${accent}28`,
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                          color: accent,
                          flexShrink: 0,
                        }}
                      >
                        {PROVIDER_ICONS[pt.id] ?? (
                          <span style={{ fontSize: 18 }}>◌</span>
                        )}
                      </div>
                      <div>
                        <div
                          style={{
                            fontWeight: 600,
                            fontSize: 14,
                            letterSpacing: "-0.01em",
                          }}
                        >
                          {pt.name}
                        </div>
                        <div
                          style={{
                            fontSize: 12,
                            color: "var(--color-text-secondary)",
                            marginTop: 2,
                          }}
                        >
                          {pt.desc}
                        </div>
                      </div>
                      <span
                        style={{
                          marginLeft: "auto",
                          color: "var(--color-text-tertiary)",
                          fontSize: 16,
                        }}
                      >
                        ›
                      </span>
                    </button>
                  )
                })}
              </div>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

// ─── Providers Page ───────────────────────────────────────────────────────────

export function ProvidersPage({ showToast }: ProvidersPageProps) {
  const [providers, setProviders] = useState<Array<Provider>>([])
  const [status, setStatus] = useState<Status | null>(null)
  const [loading, setLoading] = useState(true)
  const [activating, setActivating] = useState<string | null>(null)
  const [priorities, setPriorities] = useState<Record<string, number>>({})
  const [collapsedGroups, setCollapsedGroups] = useState<
    Record<string, boolean>
  >({})
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null)

  const load = useCallback(async () => {
    try {
      const [p, s, pri] = await Promise.all([
        listProviders(),
        getStatus(),
        getProviderPriorities(),
      ])
      setProviders(p)
      setStatus(s)
      setPriorities(pri.priorities)
    } catch (e) {
      showToast(
        "Failed to load: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setLoading(false)
    }
  }, [showToast])

  const stopPoll = useCallback(() => {
    if (pollTimer.current) {
      clearInterval(pollTimer.current)
      pollTimer.current = null
    }
  }, [])

  const startPoll = useCallback(() => {
    stopPoll()
    pollTimer.current = setInterval(async () => {
      try {
        const af = await getAuthStatus()
        setStatus((prev) => (prev ? { ...prev, authFlow: af } : prev))
        if (af?.status === "complete") {
          stopPoll()
          showToast("Authentication complete!")
          await load()
        } else if (af?.status === "error") {
          stopPoll()
          showToast("Auth failed: " + (af.error ?? "unknown"), "error")
          await load()
        }
      } catch {
        /* ignore */
      }
    }, 2000)
  }, [load, showToast, stopPoll])

  const handleCancelAuth = useCallback(async () => {
    stopPoll()
    setStatus((prev) => (prev ? { ...prev, authFlow: null } : prev))
    try {
      await cancelAuth()
    } catch {
      /* ignore */
    }
  }, [stopPoll])

  useEffect(() => {
    void load()
    return stopPoll
  }, [load, stopPoll])

  useEffect(() => {
    const authFlow = status?.authFlow
    if (
      authFlow
      && (authFlow.status === "pending" || authFlow.status === "awaiting_user")
    ) {
      startPoll()
    }
  }, [startPoll, status])

  const handleActivate = async (id: string) => {
    setActivating(id)
    try {
      const result = await activateProvider(id)
      if (result.success) {
        showToast(`Activated ${result.provider?.name ?? id}`)
        await load()
      }
    } catch (e) {
      showToast(
        "Activate failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleDeactivate = async (id: string) => {
    setActivating(id)
    try {
      await deactivateProvider(id)
      showToast(`Deactivated`)
      await load()
    } catch (e) {
      showToast(
        "Deactivate failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleDelete = async (id: string) => {
    setActivating(id)
    try {
      const result = await deleteProvider(id)
      showToast(result.message || `Deleted`)
      await load()
    } catch (e) {
      showToast(
        "Delete failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    } finally {
      setActivating(null)
    }
  }

  const handleAuthSubmit = async (id: string, body: Record<string, string>) => {
    try {
      const result = await authProvider(id, body)
      if (result.success) {
        showToast("Authentication successful")
        await load()
      } else if (result.requiresAuth) {
        showToast("Follow the auth instructions above")
        startPoll()
        await load()
      }
    } catch (e) {
      showToast(
        "Auth failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const handleAddInstance = async (providerType: string) => {
    try {
      const result = await addProviderInstance(providerType)
      if (result.success && result.provider) {
        showToast(`Created ${result.provider.name}`)
        await load()
      }
    } catch (e) {
      showToast(
        "Add failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const handlePrioritiesChange = async (
    newPriorities: Record<string, number>,
  ) => {
    try {
      await setProviderPriorities(newPriorities)
      setPriorities(newPriorities)
      showToast("Priorities updated")
    } catch (e) {
      showToast(
        "Priority update failed: "
          + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const isFlowRunning = ["awaiting_user", "pending"].includes(
    status?.authFlow?.status ?? "",
  )

  const ALL_TYPES = ["github-copilot", "antigravity", "alibaba", "azure-openai"]
  const TYPE_NAMES: Record<string, string> = {
    "github-copilot": "GitHub Copilot",
    antigravity: "Antigravity (Google)",
    alibaba: "Alibaba DashScope",
    "azure-openai": "Azure OpenAI",
  }

  const providerGroups = providers.reduce<Record<string, Array<Provider>>>(
    (g, p) => {
      if (!g[p.type]) g[p.type] = []
      g[p.type].push(p)
      return g
    },
    {},
  )

  const completeGroups: Record<string, Array<Provider>> = {}
  for (const t of ALL_TYPES) completeGroups[t] = providerGroups[t] ?? []

  // Determine if all groups are collapsed/expanded
  const groupsWithProviders = Object.entries(completeGroups).filter(
    ([_, typeProviders]) => typeProviders.length > 0,
  )
  const allCollapsed = groupsWithProviders.every(
    ([providerType, _]) => collapsedGroups[providerType] ?? false,
  )

  // Toggle all groups collapsed/expanded
  const toggleAllGroups = () => {
    const newState: Record<string, boolean> = {}
    for (const [providerType, _] of groupsWithProviders) {
      newState[providerType] = !allCollapsed
    }
    setCollapsedGroups(newState)
  }

  const activeProviders = providers
    .filter((p) => p.isActive)
    .sort((a, b) => (priorities[a.id] ?? 0) - (priorities[b.id] ?? 0))
  const totalActive = activeProviders.length

  if (loading && providers.length === 0) {
    return (
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          gap: 12,
          padding: "80px 0",
          color: "var(--color-text-secondary)",
          fontSize: 14,
        }}
      >
        <Spin size={16} /> Loading providers…
      </div>
    )
  }

  return (
    <div>
      <AuthFlowBanner authFlow={status?.authFlow} providers={providers} onCancel={handleCancelAuth} />

      {/* Page header */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          marginBottom: 28,
        }}
      >
        <div
          onClick={groupsWithProviders.length > 0 ? toggleAllGroups : undefined}
          style={{
            cursor: groupsWithProviders.length > 0 ? "pointer" : "default",
            userSelect: "none",
            display: "flex",
            alignItems: "center",
            gap: 8,
          }}
        >
          <div>
            <h1
              style={{
                fontFamily: "var(--font-display)",
                fontWeight: 700,
                fontSize: 26,
                color: "var(--color-text)",
                letterSpacing: "-0.02em",
                lineHeight: 1,
              }}
            >
              Providers
            </h1>
            <p
              style={{
                fontSize: 13,
                color: "var(--color-text-secondary)",
                marginTop: 5,
              }}
            >
              {totalActive > 0 ?
                <>
                  <span
                    style={{ color: "var(--color-green)", fontWeight: 500 }}
                  >
                    {totalActive} active
                  </span>{" "}
                  · {providers.length} total instances
                </>
              : `${providers.length} instance${providers.length !== 1 ? "s" : ""} · none active`
              }
            </p>
          </div>
        </div>
        <div style={{ display: "flex", gap: 8 }}>
          <PriorityModal
            providers={providers}
            priorities={priorities}
            onPrioritiesChange={handlePrioritiesChange}
          />
          <AddProviderModal
            onAdd={handleAddInstance}
            disabled={isFlowRunning}
          />
        </div>
      </div>

      {/* Provider groups */}
      <div style={{ display: "flex", flexDirection: "column", gap: 32 }}>
        {Object.entries(completeGroups)
          .sort(([, a], [, b]) => {
            if (a.length > 0 && b.length === 0) return -1
            if (a.length === 0 && b.length > 0) return 1
            return 0
          })
          .map(([providerType, typeProviders]) => {
          const isCollapsed = collapsedGroups[providerType] ?? false
          const accent = PROVIDER_ACCENT[providerType] ?? "#0a84ff"

          return (
            <div key={providerType}>
              {/* Group header */}
              <div
                onClick={
                  typeProviders.length > 0 ?
                    () =>
                      setCollapsedGroups((prev) => ({
                        ...prev,
                        [providerType]: !prev[providerType],
                      }))
                  : undefined
                }
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "space-between",
                  marginBottom: 12,
                  cursor: typeProviders.length > 0 ? "pointer" : "default",
                  userSelect: "none",
                }}
              >
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <div
                    style={{
                      width: 28,
                      height: 28,
                      borderRadius: "var(--radius-sm)",
                      background: `${accent}18`,
                      border: `1px solid ${accent}28`,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      color: accent,
                    }}
                  >
                    {PROVIDER_ICONS[providerType] ?? (
                      <span style={{ fontSize: 14 }}>◌</span>
                    )}
                  </div>
                  <div>
                    <span
                      style={{
                        fontFamily: "var(--font-display)",
                        fontWeight: 600,
                        fontSize: 15,
                        color: "var(--color-text)",
                        letterSpacing: "-0.01em",
                      }}
                    >
                      {TYPE_NAMES[providerType] ?? providerType}
                    </span>
                    {typeProviders.length > 0 && (
                      <span
                        style={{
                          marginLeft: 8,
                          fontSize: 12,
                          color: "var(--color-text-tertiary)",
                          fontWeight: 400,
                        }}
                      >
                        {typeProviders.length}{" "}
                        {typeProviders.length === 1 ? "account" : "accounts"}
                      </span>
                    )}
                  </div>
                </div>
              </div>

              {!isCollapsed && (
                <div
                  style={{ display: "flex", flexDirection: "column", gap: 10 }}
                >
                  {typeProviders.length > 0 ?
                    typeProviders.map((p) => (
                      <ProviderCard
                        key={p.id}
                        provider={p}
                        isFlowRunning={isFlowRunning}
                        isActivating={activating === p.id}
                        onActivate={handleActivate}
                        onDeactivate={handleDeactivate}
                        onDelete={handleDelete}
                        onAuthSubmit={handleAuthSubmit}
                        onModelsChanged={load}
                        priorityIndex={activeProviders.findIndex(
                          (x) => x.id === p.id,
                        )}
                        multiProvider={activeProviders.length >= 2}
                      />
                    ))
                  : <div
                      style={{
                        padding: "32px 24px",
                        border: `1px dashed ${accent}28`,
                        borderRadius: "var(--radius-lg)",
                        textAlign: "center",
                        background: `${accent}06`,
                      }}
                    >
                      <div
                        style={{
                          width: 44,
                          height: 44,
                          borderRadius: "var(--radius-md)",
                          background: `${accent}14`,
                          border: `1px solid ${accent}22`,
                          display: "flex",
                          alignItems: "center",
                          justifyContent: "center",
                          color: accent,
                          margin: "0 auto 12px",
                          opacity: 0.6,
                        }}
                      >
                        {PROVIDER_ICONS[providerType] ?? (
                          <span style={{ fontSize: 20 }}>◌</span>
                        )}
                      </div>
                      <p
                        style={{
                          fontSize: 13,
                          color: "var(--color-text-secondary)",
                          marginBottom: 14,
                        }}
                      >
                        No {TYPE_NAMES[providerType]} accounts configured
                      </p>
                      <button
                        className="btn btn-ghost btn-sm"
                        onClick={() => handleAddInstance(providerType)}
                        disabled={isFlowRunning}
                      >
                        Add Account
                      </button>
                    </div>
                  }
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
