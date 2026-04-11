import { useState, useEffect, useCallback } from "react"
import {
  listVirtualModels,
  listProviders,
  getProviderModels,
  createVirtualModel,
  updateVirtualModel,
  deleteVirtualModel,
  type VirtualModel,
  type VirtualModelUpstream,
  type LbStrategy,
  type Provider,
  type Model,
} from "@/api"

interface Props {
  showToast: (msg: string, type?: "success" | "error") => void
}

const LB_STRATEGIES: Array<{ value: LbStrategy; label: string; hint: string }> = [
  { value: "round-robin", label: "Round-robin", hint: "Cycle through upstreams in order" },
  { value: "random", label: "Random", hint: "Pick a random upstream each request" },
  { value: "priority", label: "Priority / failover", hint: "Highest-priority upstream first" },
  { value: "weighted", label: "Weighted", hint: "Distribute by numeric weight" },
]

const emptyForm = (): Partial<VirtualModel> => ({
  virtual_model_id: "",
  name: "",
  description: "",
  api_shape: "openai",
  lb_strategy: "round-robin",
  enabled: true,
  upstreams: [],
})

interface UpstreamRow extends VirtualModelUpstream {
  // the provider instance ID selected in the first dropdown
  selectedProviderId: string
}

const emptyUpstreamRow = (): UpstreamRow => ({
  model_id: "",
  weight: 1,
  priority: 0,
  selectedProviderId: "",
})

// eslint-disable-next-line max-lines-per-function
export function VmodelPage({ showToast }: Props) {
  const [vmodels, setVmodels] = useState<Array<VirtualModel>>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<VirtualModel | null>(null)
  const [form, setForm] = useState<Partial<VirtualModel>>(emptyForm())
  const [upstreamRows, setUpstreamRows] = useState<Array<UpstreamRow>>([emptyUpstreamRow()])
  const [saving, setSaving] = useState(false)
  const [isNew, setIsNew] = useState(false)

  // Provider + model catalogue
  const [providers, setProviders] = useState<Array<Provider>>([])
  const [providerModels, setProviderModels] = useState<Record<string, Array<Model>>>({})
  const [catalogueLoading, setCatalogueLoading] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const data = await listVirtualModels()
      setVmodels(data)
    } catch {
      showToast("Failed to load virtual models", "error")
    } finally {
      setLoading(false)
    }
  }, [showToast])

  // Load provider catalogue once
  const loadCatalogue = useCallback(async () => {
    setCatalogueLoading(true)
    try {
      const provList = await listProviders()
      setProviders(provList)
      // Fetch models for all active/authenticated providers in parallel
      const entries = await Promise.all(
        provList.map(async (p) => {
          try {
            const res = await getProviderModels(p.id)
            return [p.id, res.models ?? []] as [string, Array<Model>]
          } catch {
            return [p.id, []] as [string, Array<Model>]
          }
        }),
      )
      setProviderModels(Object.fromEntries(entries))
    } catch {
      showToast("Failed to load providers", "error")
    } finally {
      setCatalogueLoading(false)
    }
  }, [showToast])

  useEffect(() => {
    load()
    loadCatalogue()
  }, [load, loadCatalogue])

  // When a provider is selected for a row, auto-clear the model_id
  const setRowProvider = (i: number, providerId: string) => {
    setUpstreamRows((prev) =>
      prev.map((r, idx) =>
        idx === i ? { ...r, selectedProviderId: providerId, model_id: "" } : r,
      ),
    )
  }

  const setRowModel = (i: number, modelId: string) => {
    setUpstreamRows((prev) =>
      prev.map((r, idx) => (idx === i ? { ...r, model_id: modelId } : r)),
    )
  }

  const setRowNum = (i: number, field: "weight" | "priority", value: number) => {
    setUpstreamRows((prev) =>
      prev.map((r, idx) => (idx === i ? { ...r, [field]: value } : r)),
    )
  }

  const addRow = () => setUpstreamRows((prev) => [...prev, emptyUpstreamRow()])
  const removeRow = (i: number) =>
    setUpstreamRows((prev) => prev.filter((_, idx) => idx !== i))

  const openNew = () => {
    setSelected(null)
    setIsNew(true)
    setForm(emptyForm())
    setUpstreamRows([emptyUpstreamRow()])
  }

  const openEdit = (vm: VirtualModel) => {
    setSelected(vm)
    setIsNew(false)
    setForm({ ...vm })
    // Reconstruct rows; try to find the provider that owns each model
    const rows: Array<UpstreamRow> = (vm.upstreams.length ? vm.upstreams : [{ model_id: "", weight: 1, priority: 0 }]).map((u) => {
      const owningProvider = providers.find((p) =>
        (providerModels[p.id] ?? []).some((m) => m.id === u.model_id),
      )
      return { ...u, selectedProviderId: owningProvider?.id ?? "" }
    })
    setUpstreamRows(rows)
  }

  const closeForm = () => {
    setSelected(null)
    setIsNew(false)
    setForm(emptyForm())
    setUpstreamRows([emptyUpstreamRow()])
  }

  const handleSave = async () => {
    if (!form.virtual_model_id?.trim()) { showToast("Model ID is required", "error"); return }
    if (!form.name?.trim()) { showToast("Display name is required", "error"); return }
    if (!form.lb_strategy) { showToast("LB strategy is required", "error"); return }
    const filledRows = upstreamRows.filter((r) => r.model_id.trim())
    if (filledRows.length === 0) { showToast("At least one upstream model is required", "error"); return }

    setSaving(true)
    try {
      const payload = {
        virtual_model_id: form.virtual_model_id!,
        name: form.name!,
        description: form.description ?? "",
        api_shape: form.api_shape ?? "openai",
        lb_strategy: form.lb_strategy!,
        enabled: form.enabled ?? true,
        upstreams: filledRows.map((r) => ({
          model_id: r.model_id,
          weight: r.weight ?? 1,
          priority: r.priority ?? 0,
        })),
      }
      if (isNew) {
        await createVirtualModel(payload)
        showToast("Virtual model created", "success")
      } else {
        await updateVirtualModel(form.virtual_model_id!, payload)
        showToast("Virtual model updated", "success")
      }
      closeForm()
      await load()
    } catch (err: unknown) {
      showToast(err instanceof Error ? err.message : "Failed to save virtual model", "error")
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    if (!window.confirm(`Delete virtual model "${id}"?`)) return
    try {
      await deleteVirtualModel(id)
      showToast("Deleted", "success")
      if (selected?.virtual_model_id === id) closeForm()
      await load()
    } catch {
      showToast("Failed to delete", "error")
    }
  }

  const showWeight = form.lb_strategy === "weighted"
  const showPriority = form.lb_strategy === "priority"
  const isEditing = isNew || selected !== null

  return (
    <div style={{ display: "flex", gap: 24, alignItems: "flex-start" }}>
      {/* ── List panel ── */}
      <div style={{ flex: "0 0 380px", minWidth: 0 }}>
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 16 }}>
          <h2 style={{ margin: 0, fontSize: 18, fontWeight: 700, color: "var(--color-text)" }}>
            Virtual Models
          </h2>
          <button onClick={openNew} style={primaryBtnStyle}>+ New</button>
        </div>

        {loading ? (
          <div style={{ color: "var(--color-text-secondary)", fontSize: 13 }}>Loading…</div>
        ) : vmodels.length === 0 ? (
          <div style={{
            padding: 24, borderRadius: "var(--radius-lg)",
            border: "1px dashed var(--color-separator)", textAlign: "center",
            color: "var(--color-text-tertiary)", fontSize: 13,
          }}>
            No virtual models yet. Click <strong>+ New</strong> to create one.
          </div>
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {vmodels.map((vm) => (
              <div
                key={vm.virtual_model_id}
                onClick={() => openEdit(vm)}
                style={{
                  padding: "12px 16px", borderRadius: "var(--radius-lg)", cursor: "pointer",
                  border: `1px solid ${selected?.virtual_model_id === vm.virtual_model_id ? "var(--color-blue)" : "var(--color-separator)"}`,
                  background: selected?.virtual_model_id === vm.virtual_model_id ? "rgba(10,132,255,0.08)" : "var(--color-surface)",
                  transition: "border-color 0.15s",
                }}
              >
                <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
                  <div>
                    <div style={{ fontWeight: 600, fontSize: 13, color: "var(--color-text)", fontFamily: "var(--font-mono)" }}>
                      {vm.virtual_model_id}
                    </div>
                    <div style={{ fontSize: 12, color: "var(--color-text-secondary)", marginTop: 2 }}>
                      {vm.name} · {vm.lb_strategy} · {vm.upstreams.length} upstream{vm.upstreams.length !== 1 ? "s" : ""}
                    </div>
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    <span style={{
                      fontSize: 11, padding: "2px 8px", borderRadius: 999, fontWeight: 600,
                      background: vm.enabled ? "rgba(52,199,89,0.15)" : "rgba(142,142,147,0.15)",
                      color: vm.enabled ? "var(--color-green)" : "var(--color-text-tertiary)",
                    }}>
                      {vm.enabled ? "enabled" : "disabled"}
                    </span>
                    <button
                      onClick={(e) => { e.stopPropagation(); handleDelete(vm.virtual_model_id) }}
                      style={{ background: "none", border: "none", color: "var(--color-text-tertiary)", cursor: "pointer", padding: "2px 4px", fontSize: 14 }}
                    >✕</button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* ── Edit / Create panel ── */}
      {isEditing && (
        <div style={{
          flex: 1, minWidth: 0, padding: 24, borderRadius: "var(--radius-lg)",
          border: "1px solid var(--color-separator)", background: "var(--color-surface)",
        }}>
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 20 }}>
            <h3 style={{ margin: 0, fontSize: 16, fontWeight: 700, color: "var(--color-text)" }}>
              {isNew ? "New Virtual Model" : `Edit: ${selected?.virtual_model_id}`}
            </h3>
            <button onClick={closeForm} style={{ background: "none", border: "none", color: "var(--color-text-tertiary)", cursor: "pointer", fontSize: 18 }}>✕</button>
          </div>

          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            {/* Model ID */}
            <Field label="Model ID" hint="Unique identifier exposed via /v1/models">
              <input
                value={form.virtual_model_id ?? ""}
                onChange={(e) => setForm((f) => ({ ...f, virtual_model_id: e.target.value }))}
                disabled={!isNew}
                placeholder="e.g. claude-mythos-5.0"
                style={inputStyle(!isNew)}
              />
            </Field>

            {/* Display name */}
            <Field label="Display name">
              <input
                value={form.name ?? ""}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                placeholder="My Virtual Model"
                style={inputStyle()}
              />
            </Field>

            {/* Description */}
            <Field label="Description" hint="Optional">
              <input
                value={form.description ?? ""}
                onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                placeholder="What this virtual model does"
                style={inputStyle()}
              />
            </Field>

            {/* LB strategy */}
            <Field label="Load-balancing strategy">
              <select
                value={form.lb_strategy ?? "round-robin"}
                onChange={(e) => setForm((f) => ({ ...f, lb_strategy: e.target.value as LbStrategy }))}
                style={inputStyle()}
              >
                {LB_STRATEGIES.map((s) => (
                  <option key={s.value} value={s.value}>{s.label} — {s.hint}</option>
                ))}
              </select>
            </Field>

            {/* Enabled */}
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <label style={{ fontSize: 13, color: "var(--color-text-secondary)", fontWeight: 500 }}>Enabled</label>
              <input
                type="checkbox"
                checked={form.enabled ?? true}
                onChange={(e) => setForm((f) => ({ ...f, enabled: e.target.checked }))}
                style={{ width: 16, height: 16, cursor: "pointer" }}
              />
            </div>

            {/* Upstreams */}
            <div>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8 }}>
                <span style={{ fontSize: 13, fontWeight: 600, color: "var(--color-text)" }}>Upstream models</span>
                <button onClick={addRow} style={addBtnStyle}>+ Add upstream</button>
              </div>

              {/* Column headers */}
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr" + (showWeight || showPriority ? " 90px" : "") + " 28px", gap: 6, marginBottom: 4, padding: "0 2px" }}>
                <div style={colHeaderStyle}>Provider</div>
                <div style={colHeaderStyle}>Model</div>
                {showWeight && <div style={colHeaderStyle}>Weight</div>}
                {showPriority && <div style={colHeaderStyle}>Priority</div>}
                <div />
              </div>

              {catalogueLoading && (
                <div style={{ fontSize: 12, color: "var(--color-text-tertiary)", marginBottom: 8 }}>
                  Loading providers…
                </div>
              )}

              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {upstreamRows.map((row, i) => {
                  const availableModels = row.selectedProviderId
                    ? (providerModels[row.selectedProviderId] ?? [])
                    : []

                  return (
                    <div
                      key={i}
                      style={{ display: "grid", gridTemplateColumns: "1fr 1fr" + (showWeight || showPriority ? " 90px" : "") + " 28px", gap: 6, alignItems: "center" }}
                    >
                      {/* Provider dropdown */}
                      <select
                        value={row.selectedProviderId}
                        onChange={(e) => setRowProvider(i, e.target.value)}
                        style={inputStyle()}
                      >
                        <option value="">— provider —</option>
                        {providers.map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name || p.id}
                          </option>
                        ))}
                      </select>

                      {/* Model dropdown */}
                      <select
                        value={row.model_id}
                        onChange={(e) => setRowModel(i, e.target.value)}
                        disabled={!row.selectedProviderId}
                        style={inputStyle(!row.selectedProviderId)}
                      >
                        <option value="">— model —</option>
                        {availableModels.map((m) => (
                          <option key={m.id} value={m.id}>
                            {m.name || m.id}
                          </option>
                        ))}
                      </select>

                      {showWeight && (
                        <input
                          type="number"
                          min={1}
                          value={row.weight}
                          onChange={(e) => setRowNum(i, "weight", Number(e.target.value))}
                          title="Weight"
                          style={inputStyle()}
                        />
                      )}
                      {showPriority && (
                        <input
                          type="number"
                          min={0}
                          value={row.priority}
                          onChange={(e) => setRowNum(i, "priority", Number(e.target.value))}
                          title="Priority (lower = higher priority)"
                          style={inputStyle()}
                        />
                      )}

                      <button
                        onClick={() => removeRow(i)}
                        disabled={upstreamRows.length === 1}
                        style={{
                          background: "none", border: "none", padding: "0 4px", fontSize: 16,
                          color: upstreamRows.length === 1 ? "var(--color-separator)" : "var(--color-red, #ff453a)",
                          cursor: upstreamRows.length === 1 ? "default" : "pointer",
                        }}
                      >✕</button>
                    </div>
                  )
                })}
              </div>

              {(showWeight || showPriority) && (
                <div style={{ fontSize: 11, color: "var(--color-text-tertiary)", marginTop: 6 }}>
                  {showWeight && "Weight: higher value = more traffic.  "}
                  {showPriority && "Priority: lower number = higher priority (0 = first choice)."}
                </div>
              )}
            </div>

            {/* Actions */}
            <div style={{ display: "flex", gap: 10, marginTop: 8 }}>
              <button onClick={handleSave} disabled={saving} style={{ ...primaryBtnStyle, opacity: saving ? 0.6 : 1, cursor: saving ? "default" : "pointer" }}>
                {saving ? "Saving…" : isNew ? "Create" : "Save"}
              </button>
              {!isNew && selected && (
                <button
                  onClick={() => handleDelete(selected.virtual_model_id)}
                  style={{
                    padding: "8px 20px", borderRadius: "var(--radius-md)", fontSize: 13, fontWeight: 600, cursor: "pointer",
                    background: "rgba(255,69,58,0.12)", color: "var(--color-red, #ff453a)",
                    border: "1px solid rgba(255,69,58,0.2)",
                  }}
                >
                  Delete
                </button>
              )}
              <button
                onClick={closeForm}
                style={{
                  padding: "8px 16px", borderRadius: "var(--radius-md)", fontSize: 13, cursor: "pointer",
                  background: "transparent", color: "var(--color-text-secondary)",
                  border: "1px solid var(--color-separator)",
                }}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div>
      <label style={{
        display: "block", fontSize: 12, fontWeight: 600, textTransform: "uppercase",
        letterSpacing: "0.04em", color: "var(--color-text-secondary)", marginBottom: 4,
      }}>
        {label}
        {hint && (
          <span style={{ fontWeight: 400, textTransform: "none", letterSpacing: 0, marginLeft: 6, color: "var(--color-text-tertiary)" }}>
            — {hint}
          </span>
        )}
      </label>
      {children}
    </div>
  )
}

function inputStyle(disabled = false): React.CSSProperties {
  return {
    width: "100%", boxSizing: "border-box", padding: "7px 10px",
    borderRadius: "var(--radius-md)", border: "1px solid var(--color-separator)",
    background: disabled ? "var(--color-surface-2, rgba(255,255,255,0.04))" : "var(--color-surface)",
    color: disabled ? "var(--color-text-tertiary)" : "var(--color-text)",
    fontSize: 13, fontFamily: "var(--font-text)", outline: "none",
    opacity: disabled ? 0.6 : 1,
  }
}

const primaryBtnStyle: React.CSSProperties = {
  padding: "8px 20px", borderRadius: "var(--radius-md)",
  background: "var(--color-blue)", color: "#fff",
  border: "none", fontSize: 13, fontWeight: 600, cursor: "pointer",
}

const addBtnStyle: React.CSSProperties = {
  padding: "4px 10px", borderRadius: "var(--radius-sm)",
  background: "rgba(10,132,255,0.12)", color: "var(--color-blue)",
  border: "1px solid rgba(10,132,255,0.2)", fontSize: 12, fontWeight: 600, cursor: "pointer",
}

const colHeaderStyle: React.CSSProperties = {
  fontSize: 11, fontWeight: 600, color: "var(--color-text-tertiary)",
  textTransform: "uppercase", letterSpacing: "0.04em",
}
