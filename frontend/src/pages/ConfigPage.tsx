import { CSSProperties, useEffect, useState } from "react"
import { listConfigFiles, getConfigFile, saveConfigFile, type ConfigFileEntry } from "@/api"
import {
  Save,
  RotateCcw,
  Plus,
  Trash2,
  ChevronDown,
  ChevronRight,
  FileJson,
  FileText,
  Settings2,
  Key,
  Plug,
  Code,
} from "lucide-react"

interface ConfigPageProps {
  showToast: (msg: string, type?: "success" | "error") => void
}

// ─── Claude Code Settings types ──────────────────────────────────────────────

interface ClaudeCodeSettings {
  model?: string
  env?: Record<string, string>
  enabledPlugins?: Record<string, boolean>
  extraKnownMarketplaces?: Record<string, unknown>
  autoUpdatesChannel?: string
  skipDangerousModePermissionPrompt?: boolean
  [key: string]: unknown
}

// ─── Codex Config types ──────────────────────────────────────────────────────

interface CodexModelProvider {
  name: string
  base_url: string
  env_key: string
}

interface CodexProfile {
  name: string
  model: string
  model_provider: string
  model_reasoning_effort: string
  sandbox: string
}

interface CodexConfig {
  model?: string
  model_reasoning_effort?: string
  profile?: string
  model_providers?: Record<string, CodexModelProvider>
  profiles?: Record<string, CodexProfile>
  projects?: Record<string, { trust_level: string }>
  [key: string]: unknown
}

// ─── Minimal TOML parser ─────────────────────────────────────────────────────

function parseTOML(text: string): CodexConfig {
  const result: CodexConfig = {}
  let currentSection: string | null = null

  for (const rawLine of text.split("\n")) {
    const line = rawLine.trim()
    if (!line || line.startsWith("#")) continue

    // Sub-section like [profiles.xxx.windows] — skip subsections
    const multiDotMatch = line.match(/^\[([^\]]+\.[^\]]+\.[^\]]+)\]$/)
    if (multiDotMatch) {
      currentSection = "__skip__"
      continue
    }

    const sectionMatch = line.match(/^\[([^\]]+)\]$/)
    if (sectionMatch) {
      const raw = sectionMatch[1].replace(/^["']|["']$/g, "")
      currentSection = raw

      if (raw.startsWith("model_providers.")) {
        const key = raw.replace("model_providers.", "").replace(/^["']|["']$/g, "")
        if (!result.model_providers) result.model_providers = {}
        result.model_providers[key] = { name: key, base_url: "", env_key: "" }
      } else if (raw.startsWith("profiles.")) {
        const key = raw.replace("profiles.", "").replace(/^["']|["']$/g, "")
        if (!result.profiles) result.profiles = {}
        result.profiles[key] = { name: key, model: "", model_provider: "", model_reasoning_effort: "", sandbox: "" }
      } else if (raw.startsWith("projects.")) {
        const key = raw.replace("projects.", "").replace(/^["']|["']$/g, "")
        if (!result.projects) result.projects = {}
        result.projects[key] = { trust_level: "" }
      }
      continue
    }

    if (currentSection === "__skip__") continue

    const kvMatch = line.match(/^([\w.]+)\s*=\s*(.+)$/)
    if (!kvMatch) continue

    const key = kvMatch[1]
    let value = kvMatch[2].trim().split(" #")[0].trim()

    let parsed: string | boolean = value
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      parsed = value.slice(1, -1)
    } else if (value === "true") {
      parsed = true
    } else if (value === "false") {
      parsed = false
    }

    const s = String(parsed)

    if (!currentSection) {
      (result as Record<string, unknown>)[key] = parsed
    } else if (currentSection.startsWith("model_providers.")) {
      const name = currentSection.replace("model_providers.", "").replace(/^["']|["']$/g, "")
      if (result.model_providers?.[name]) (result.model_providers[name] as Record<string, string>)[key] = s
    } else if (currentSection.startsWith("profiles.")) {
      const name = currentSection.replace("profiles.", "").replace(/^["']|["']$/g, "")
      if (result.profiles?.[name]) (result.profiles[name] as Record<string, string>)[key] = s
    } else if (currentSection.startsWith("projects.")) {
      const name = currentSection.replace("projects.", "").replace(/^["']|["']$/g, "")
      if (result.projects?.[name]) result.projects[name][key as "trust_level"] = s
    }
  }

  return result
}

function serializeTOML(config: CodexConfig, originalContent: string): string {
  // For TOML we do a targeted replacement strategy to preserve the original structure
  // but update the values we track in structured fields
  let lines = originalContent.split("\n")
  const result: string[] = []
  let currentSection = ""

  for (const line of lines) {
    const sectionMatch = line.trim().match(/^\[([^\]]+)\]$/)
    if (sectionMatch) {
      currentSection = sectionMatch[1].replace(/^["']|["']$/g, "")
      result.push(line)
      continue
    }

    const kvMatch = line.trim().match(/^([\w.]+)\s*=\s*(.+)$/)
    if (kvMatch) {
      const key = kvMatch[1]

      if (!currentSection) {
        if (key === "model" && config.model !== undefined) {
          result.push(`${key} = "${config.model}"`)
          continue
        }
        if (key === "model_reasoning_effort" && config.model_reasoning_effort !== undefined) {
          result.push(`${key} = "${config.model_reasoning_effort}"`)
          continue
        }
        if (key === "profile" && config.profile !== undefined) {
          result.push(`${key} = '${config.profile}'`)
          continue
        }
      } else if (currentSection.startsWith("model_providers.")) {
        const name = currentSection.replace("model_providers.", "").replace(/^["']|["']$/g, "")
        const provider = config.model_providers?.[name]
        if (provider) {
          if (key === "base_url") { result.push(`base_url = "${provider.base_url}"`); continue }
          if (key === "env_key") { result.push(`env_key = "${provider.env_key}"`); continue }
          if (key === "name") { result.push(`name = "${provider.name}"`); continue }
        }
      } else if (currentSection.startsWith("profiles.")) {
        const name = currentSection.replace("profiles.", "").replace(/^["']|["']$/g, "")
        const profile = config.profiles?.[name]
        if (profile) {
          if (key === "model") { result.push(`model = "${profile.model}"`); continue }
          if (key === "model_provider") { result.push(`model_provider = "${profile.model_provider}"`); continue }
          if (key === "model_reasoning_effort") { result.push(`model_reasoning_effort = "${profile.model_reasoning_effort}"`); continue }
          if (key === "sandbox") { result.push(`sandbox = "${profile.sandbox}"`); continue }
        }
      } else if (currentSection.startsWith("projects.")) {
        const name = currentSection.replace("projects.", "").replace(/^["']|["']$/g, "")
        const project = config.projects?.[name]
        if (project && key === "trust_level") {
          result.push(`trust_level = "${project.trust_level}"`)
          continue
        }
      }
    }

    result.push(line)
  }

  return result.join("\n")
}

// ─── Collapsible Section ─────────────────────────────────────────────────────

function Section({
  title,
  icon: Icon,
  count,
  children,
  defaultOpen = true,
}: {
  title: string
  icon?: React.ElementType
  count?: number
  children: React.ReactNode
  defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div style={{ marginBottom: 12 }}>
      <button
        onClick={() => setOpen(!open)}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          width: "100%",
          padding: "9px 12px",
          background: "var(--color-surface)",
          border: "1px solid var(--color-separator)",
          borderRadius: "var(--radius-md)",
          borderBottomLeftRadius: open ? 0 : "var(--radius-md)",
          borderBottomRightRadius: open ? 0 : "var(--radius-md)",
          color: "var(--color-text)",
          fontSize: 12,
          fontWeight: 600,
          cursor: "pointer",
          textAlign: "left",
        }}
      >
        {Icon && <Icon size={13} style={{ color: "var(--color-blue)" }} />}
        {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
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
      </button>
      {open && (
        <div
          style={{
            padding: 14,
            border: "1px solid var(--color-separator)",
            borderTop: "none",
            borderBottomLeftRadius: "var(--radius-md)",
            borderBottomRightRadius: "var(--radius-md)",
            background: "var(--color-bg-elevated)",
          }}
        >
          {children}
        </div>
      )}
    </div>
  )
}

// ─── Field row ───────────────────────────────────────────────────────────────

function Field({
  label,
  children,
  labelWidth = 160,
}: {
  label: string
  children: React.ReactNode
  labelWidth?: number
}) {
  return (
    <div style={{ display: "flex", gap: 12, alignItems: "center", marginBottom: 10 }}>
      <label
        style={{
          fontSize: 12,
          color: "var(--color-text-secondary)",
          minWidth: labelWidth,
          flexShrink: 0,
        }}
      >
        {label}
      </label>
      {children}
    </div>
  )
}

const inputStyle: CSSProperties = {
  flex: 1,
  padding: "6px 10px",
  borderRadius: "var(--radius-sm)",
  border: "1px solid var(--color-separator)",
  background: "var(--color-surface)",
  color: "var(--color-text)",
  fontSize: 12,
  fontFamily: "var(--font-mono)",
}

const smallInputStyle: CSSProperties = {
  ...inputStyle,
  padding: "4px 8px",
  fontSize: 11,
  background: "var(--color-bg-elevated)",
}

// ─── Claude Code Editor ───────────────────────────────────────────────────────

function ClaudeCodeEditor({
  settings,
  onChange,
}: {
  settings: ClaudeCodeSettings
  onChange: (s: ClaudeCodeSettings) => void
}) {
  const envEntries = Object.entries(settings.env ?? {})
  const pluginEntries = Object.entries(settings.enabledPlugins ?? {})

  const setEnvKey = (oldKey: string, newKey: string) => {
    const env = { ...settings.env }
    const val = env[oldKey]
    delete env[oldKey]
    env[newKey] = val
    onChange({ ...settings, env })
  }

  const setEnvVal = (key: string, val: string) => {
    onChange({ ...settings, env: { ...settings.env, [key]: val } })
  }

  const deleteEnv = (key: string) => {
    const env = { ...settings.env }
    delete env[key]
    onChange({ ...settings, env })
  }

  return (
    <div>
      {/* Model */}
      <Section title="Default Model" icon={Settings2}>
        <Field label="Model">
          <input
            value={settings.model ?? ""}
            onChange={(e) => onChange({ ...settings, model: e.target.value })}
            style={inputStyle}
            placeholder="e.g. claude-opus-4-5"
          />
        </Field>
        <Field label="Auto Updates Channel">
          <input
            value={settings.autoUpdatesChannel ?? ""}
            onChange={(e) => onChange({ ...settings, autoUpdatesChannel: e.target.value })}
            style={inputStyle}
            placeholder="latest"
          />
        </Field>
        <label
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            fontSize: 12,
            color: "var(--color-text-secondary)",
            cursor: "pointer",
          }}
        >
          <input
            type="checkbox"
            checked={!!settings.skipDangerousModePermissionPrompt}
            onChange={(e) =>
              onChange({ ...settings, skipDangerousModePermissionPrompt: e.target.checked })
            }
            style={{ accentColor: "var(--color-blue)" }}
          />
          Skip dangerous mode permission prompt
        </label>
      </Section>

      {/* Env vars */}
      <Section title="Environment Variables" icon={Key} count={envEntries.length}>
        {envEntries.map(([key, value]) => (
          <div key={key} style={{ display: "flex", gap: 6, marginBottom: 6, alignItems: "center" }}>
            <input
              value={key}
              onChange={(e) => setEnvKey(key, e.target.value)}
              placeholder="VAR_NAME"
              style={{ ...smallInputStyle, width: 260, flex: "none" }}
            />
            <input
              value={value}
              onChange={(e) => setEnvVal(key, e.target.value)}
              placeholder="value"
              style={{ ...smallInputStyle, flex: 1 }}
            />
            <button
              onClick={() => deleteEnv(key)}
              style={{
                padding: 4,
                border: "none",
                background: "transparent",
                color: "var(--color-red, #f87171)",
                cursor: "pointer",
              }}
            >
              <Trash2 size={13} />
            </button>
          </div>
        ))}
        <button
          onClick={() =>
            onChange({ ...settings, env: { ...settings.env, "NEW_VAR": "" } })
          }
          style={{
            display: "flex",
            alignItems: "center",
            gap: 5,
            padding: "5px 10px",
            borderRadius: "var(--radius-sm)",
            border: "1px dashed var(--color-separator)",
            background: "transparent",
            color: "var(--color-text-tertiary)",
            fontSize: 11,
            cursor: "pointer",
            marginTop: 4,
          }}
        >
          <Plus size={11} /> Add variable
        </button>
      </Section>

      {/* Plugins */}
      <Section title="Plugins" icon={Plug} count={pluginEntries.length} defaultOpen={false}>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "2px 24px" }}>
          {pluginEntries.map(([plugin, enabled]) => (
            <label
              key={plugin}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 7,
                padding: "4px 0",
                cursor: "pointer",
              }}
            >
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) =>
                  onChange({
                    ...settings,
                    enabledPlugins: { ...settings.enabledPlugins, [plugin]: e.target.checked },
                  })
                }
                style={{ accentColor: "var(--color-blue)", flexShrink: 0 }}
              />
              <span
                style={{
                  fontSize: 11,
                  fontFamily: "var(--font-mono)",
                  color: enabled ? "var(--color-text)" : "var(--color-text-tertiary)",
                }}
              >
                {plugin}
              </span>
            </label>
          ))}
        </div>
      </Section>
    </div>
  )
}

// ─── Codex Editor ─────────────────────────────────────────────────────────────

function CodexEditor({
  config,
  onChange,
}: {
  config: CodexConfig
  onChange: (c: CodexConfig) => void
}) {
  const providers = Object.entries(config.model_providers ?? {})
  const profiles = Object.entries(config.profiles ?? {})
  const projects = Object.entries(config.projects ?? {})

  const reasoningOptions = ["low", "medium", "high", "xhigh"]

  return (
    <div>
      {/* Global */}
      <Section title="Global Settings" icon={Settings2}>
        <Field label="Default Model">
          <input
            value={config.model ?? ""}
            onChange={(e) => onChange({ ...config, model: e.target.value })}
            style={inputStyle}
            placeholder="e.g. gpt-5.4"
          />
        </Field>
        <Field label="Reasoning Effort">
          <select
            value={config.model_reasoning_effort ?? ""}
            onChange={(e) => onChange({ ...config, model_reasoning_effort: e.target.value })}
            style={{ ...inputStyle, fontFamily: "var(--font-text)" }}
          >
            <option value="">(none)</option>
            {reasoningOptions.map((o) => (
              <option key={o} value={o}>{o}</option>
            ))}
          </select>
        </Field>
        <Field label="Active Profile">
          <input
            value={config.profile ?? ""}
            onChange={(e) => onChange({ ...config, profile: e.target.value })}
            style={inputStyle}
            placeholder="profile name"
          />
        </Field>
      </Section>

      {/* Model providers */}
      <Section title="Model Providers" icon={Plug} count={providers.length}>
        {providers.map(([name, provider]) => (
          <div
            key={name}
            style={{
              marginBottom: 10,
              padding: 10,
              borderRadius: "var(--radius-sm)",
              border: "1px solid var(--color-separator)",
              background: "var(--color-surface)",
            }}
          >
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                marginBottom: 8,
              }}
            >
              <span
                style={{
                  fontSize: 12,
                  fontWeight: 600,
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-blue)",
                }}
              >
                {name}
              </span>
              <button
                onClick={() => {
                  const mp = { ...config.model_providers }
                  delete mp[name]
                  onChange({ ...config, model_providers: mp })
                }}
                style={{
                  padding: 2,
                  border: "none",
                  background: "transparent",
                  color: "var(--color-red, #f87171)",
                  cursor: "pointer",
                }}
              >
                <Trash2 size={12} />
              </button>
            </div>
            {(["base_url", "env_key"] as const).map((field) => (
              <div
                key={field}
                style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 5 }}
              >
                <span
                  style={{
                    fontSize: 11,
                    color: "var(--color-text-tertiary)",
                    minWidth: 60,
                  }}
                >
                  {field}
                </span>
                <input
                  value={provider[field]}
                  onChange={(e) =>
                    onChange({
                      ...config,
                      model_providers: {
                        ...config.model_providers,
                        [name]: { ...provider, [field]: e.target.value },
                      },
                    })
                  }
                  style={smallInputStyle}
                />
              </div>
            ))}
          </div>
        ))}
        <button
          onClick={() => {
            const key = "new-provider"
            onChange({
              ...config,
              model_providers: {
                ...config.model_providers,
                [key]: { name: key, base_url: "", env_key: "" },
              },
            })
          }}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 5,
            padding: "5px 10px",
            borderRadius: "var(--radius-sm)",
            border: "1px dashed var(--color-separator)",
            background: "transparent",
            color: "var(--color-text-tertiary)",
            fontSize: 11,
            cursor: "pointer",
            marginTop: 4,
          }}
        >
          <Plus size={11} /> Add provider
        </button>
      </Section>

      {/* Profiles */}
      <Section title="Profiles" icon={Settings2} count={profiles.length} defaultOpen={false}>
        {profiles.map(([name, profile]) => (
          <div
            key={name}
            style={{
              marginBottom: 10,
              padding: 10,
              borderRadius: "var(--radius-sm)",
              border: "1px solid var(--color-separator)",
              background: "var(--color-surface)",
            }}
          >
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                marginBottom: 8,
              }}
            >
              <span
                style={{
                  fontSize: 12,
                  fontWeight: 600,
                  fontFamily: "var(--font-mono)",
                  color: "var(--color-blue)",
                }}
              >
                {name}
              </span>
              <button
                onClick={() => {
                  const p = { ...config.profiles }
                  delete p[name]
                  onChange({ ...config, profiles: p })
                }}
                style={{
                  padding: 2,
                  border: "none",
                  background: "transparent",
                  color: "var(--color-red, #f87171)",
                  cursor: "pointer",
                }}
              >
                <Trash2 size={12} />
              </button>
            </div>
            {(["model", "model_provider", "sandbox"] as const).map((field) => (
              <div
                key={field}
                style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 5 }}
              >
                <span style={{ fontSize: 11, color: "var(--color-text-tertiary)", minWidth: 80 }}>
                  {field}
                </span>
                <input
                  value={profile[field]}
                  onChange={(e) =>
                    onChange({
                      ...config,
                      profiles: {
                        ...config.profiles,
                        [name]: { ...profile, [field]: e.target.value },
                      },
                    })
                  }
                  style={smallInputStyle}
                />
              </div>
            ))}
            <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 5 }}>
              <span style={{ fontSize: 11, color: "var(--color-text-tertiary)", minWidth: 80 }}>
                reasoning
              </span>
              <select
                value={profile.model_reasoning_effort}
                onChange={(e) =>
                  onChange({
                    ...config,
                    profiles: {
                      ...config.profiles,
                      [name]: { ...profile, model_reasoning_effort: e.target.value },
                    },
                  })
                }
                style={{ ...smallInputStyle, fontFamily: "var(--font-text)" }}
              >
                <option value="">(none)</option>
                {reasoningOptions.map((o) => (
                  <option key={o} value={o}>{o}</option>
                ))}
              </select>
            </div>
          </div>
        ))}
        <button
          onClick={() => {
            const key = "new-profile"
            onChange({
              ...config,
              profiles: {
                ...config.profiles,
                [key]: { name: key, model: "", model_provider: "", model_reasoning_effort: "", sandbox: "" },
              },
            })
          }}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 5,
            padding: "5px 10px",
            borderRadius: "var(--radius-sm)",
            border: "1px dashed var(--color-separator)",
            background: "transparent",
            color: "var(--color-text-tertiary)",
            fontSize: 11,
            cursor: "pointer",
            marginTop: 4,
          }}
        >
          <Plus size={11} /> Add profile
        </button>
      </Section>

      {/* Projects */}
      <Section title="Projects" icon={Settings2} count={projects.length} defaultOpen={false}>
        {projects.map(([path, project]) => (
          <div
            key={path}
            style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 6 }}
          >
            <span
              style={{
                flex: 1,
                fontSize: 11,
                fontFamily: "var(--font-mono)",
                color: "var(--color-text-secondary)",
                overflow: "hidden",
                textOverflow: "ellipsis",
                whiteSpace: "nowrap",
              }}
              title={path}
            >
              {path}
            </span>
            <select
              value={project.trust_level}
              onChange={(e) =>
                onChange({
                  ...config,
                  projects: {
                    ...config.projects,
                    [path]: { trust_level: e.target.value },
                  },
                })
              }
              style={{
                padding: "3px 8px",
                borderRadius: "var(--radius-sm)",
                border: "1px solid var(--color-separator)",
                background: "var(--color-surface)",
                color: "var(--color-text)",
                fontSize: 11,
              }}
            >
              <option value="trusted">trusted</option>
              <option value="untrusted">untrusted</option>
            </select>
            <button
              onClick={() => {
                const p = { ...config.projects }
                delete p[path]
                onChange({ ...config, projects: p })
              }}
              style={{
                padding: 2,
                border: "none",
                background: "transparent",
                color: "var(--color-red, #f87171)",
                cursor: "pointer",
              }}
            >
              <Trash2 size={12} />
            </button>
          </div>
        ))}
      </Section>
    </div>
  )
}

// ─── Tool Card ────────────────────────────────────────────────────────────────

function ToolCard({
  entry,
  isActive,
  onClick,
}: {
  entry: ConfigFileEntry
  isActive: boolean
  onClick: () => void
}) {
  const Icon = entry.language === "json" ? FileJson : FileText
  const desc = entry.language === "json"
    ? "~/.claude/settings.json"
    : "~/.codex/config.toml"

  return (
    <button
      onClick={onClick}
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 8,
        padding: "16px 18px",
        borderRadius: "var(--radius-lg)",
        border: isActive
          ? "2px solid var(--color-blue)"
          : "1px solid var(--color-separator)",
        background: isActive ? "var(--color-blue-fill)" : "var(--color-bg-elevated)",
        boxShadow: isActive ? "0 0 0 3px rgba(56,189,248,0.12)" : "var(--shadow-card)",
        cursor: "pointer",
        textAlign: "left",
        minWidth: 220,
        transition: "all 0.15s var(--ease)",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <div
          style={{
            width: 34,
            height: 34,
            borderRadius: "var(--radius-md)",
            background: isActive ? "var(--color-blue)" : "var(--color-surface-2)",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            flexShrink: 0,
          }}
        >
          <Icon size={16} color={isActive ? "white" : "var(--color-text-secondary)"} />
        </div>
        <div>
          <div
            style={{
              fontSize: 13,
              fontWeight: 700,
              color: isActive ? "var(--color-blue)" : "var(--color-text)",
              letterSpacing: "-0.01em",
            }}
          >
            {entry.label}
          </div>
          <div
            style={{
              fontSize: 11,
              fontFamily: "var(--font-mono)",
              color: "var(--color-text-tertiary)",
              marginTop: 2,
            }}
          >
            {desc}
          </div>
        </div>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <span
          style={{
            fontSize: 10,
            padding: "2px 7px",
            borderRadius: 999,
            background: entry.exists
              ? "rgba(74, 222, 128, 0.12)"
              : "var(--color-surface-2)",
            color: entry.exists ? "var(--color-green)" : "var(--color-text-tertiary)",
            fontWeight: 600,
            border: `1px solid ${entry.exists ? "rgba(74,222,128,0.3)" : "var(--color-separator)"}`,
          }}
        >
          {entry.exists ? "● exists" : "○ new"}
        </span>
        <span
          style={{
            fontSize: 10,
            padding: "2px 7px",
            borderRadius: 999,
            background: "var(--color-surface-2)",
            color: "var(--color-text-tertiary)",
            border: "1px solid var(--color-separator)",
          }}
        >
          {entry.language.toUpperCase()}
        </span>
      </div>
    </button>
  )
}

// ─── Main Page ────────────────────────────────────────────────────────────────

export function ConfigPage({ showToast }: ConfigPageProps) {
  const [configs, setConfigs] = useState<ConfigFileEntry[]>([])
  const [activeConfig, setActiveConfig] = useState<string | null>(null)
  const [rawContent, setRawContent] = useState("")
  const [originalContent, setOriginalContent] = useState("")
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const [viewMode, setViewMode] = useState<"structured" | "raw">("structured")

  const [claudeSettings, setClaudeSettings] = useState<ClaudeCodeSettings | null>(null)
  const [codexConfig, setCodexConfig] = useState<CodexConfig | null>(null)

  useEffect(() => {
    listConfigFiles()
      .then((r) => {
        setConfigs(r.configs)
        if (r.configs.length > 0) setActiveConfig(r.configs[0].name)
      })
      .catch(() => showToast("Failed to load config list", "error"))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    if (!activeConfig) return
    setLoading(true)
    setDirty(false)
    setViewMode("structured")
    getConfigFile(activeConfig)
      .then((resp) => {
        setRawContent(resp.content)
        setOriginalContent(resp.content)
        if (activeConfig === "claude-code" && resp.content) {
          try { setClaudeSettings(JSON.parse(resp.content)) } catch { setClaudeSettings(null) }
        } else if (activeConfig === "codex" && resp.content) {
          try { setCodexConfig(parseTOML(resp.content)) } catch { setCodexConfig(null) }
        }
      })
      .catch(() => showToast("Failed to load config", "error"))
      .finally(() => setLoading(false))
  }, [activeConfig])

  const getContentToSave = () => {
    if (viewMode === "raw") return rawContent
    if (activeConfig === "claude-code" && claudeSettings)
      return JSON.stringify(claudeSettings, null, 2) + "\n"
    if (activeConfig === "codex" && codexConfig)
      return serializeTOML(codexConfig, originalContent)
    return rawContent
  }

  const handleSave = () => {
    if (!activeConfig) return
    const content = getContentToSave()
    setSaving(true)
    saveConfigFile(activeConfig, content)
      .then(() => {
        setOriginalContent(content)
        setDirty(false)
        showToast("Configuration saved", "success")
      })
      .catch((err: Error) => showToast(`Save failed: ${err.message}`, "error"))
      .finally(() => setSaving(false))
  }

  const handleReset = () => {
    setRawContent(originalContent)
    setDirty(false)
    if (activeConfig === "claude-code" && originalContent) {
      try { setClaudeSettings(JSON.parse(originalContent)) } catch { /* ignore */ }
    } else if (activeConfig === "codex" && originalContent) {
      try { setCodexConfig(parseTOML(originalContent)) } catch { /* ignore */ }
    }
  }

  const handleCardClick = (name: string) => {
    if (name === activeConfig) return
    setActiveConfig(name)
  }

  const markDirty = () => setDirty(true)

  const activeEntry = configs.find((c) => c.name === activeConfig)
  const showStructured =
    viewMode === "structured" &&
    ((activeConfig === "claude-code" && claudeSettings != null) ||
      (activeConfig === "codex" && codexConfig != null))

  return (
    <div>
      {/* Page heading */}
      <div style={{ marginBottom: 24 }}>
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
          ToolConfig
        </h1>
        <p style={{ fontSize: 14, color: "var(--color-text-secondary)", margin: "8px 0 0" }}>
          Select a tool to view and edit its configuration
        </p>
      </div>

      {/* Tool cards */}
      <div style={{ display: "flex", gap: 14, flexWrap: "wrap", marginBottom: 24 }}>
        {configs.map((cfg) => (
          <ToolCard
            key={cfg.name}
            entry={cfg}
            isActive={cfg.name === activeConfig}
            onClick={() => handleCardClick(cfg.name)}
          />
        ))}
      </div>

      {/* Editor panel */}
      {activeEntry && (
        <div
          style={{
            background: "var(--color-bg-elevated)",
            borderRadius: "var(--radius-lg)",
            border: "1px solid var(--color-separator)",
            boxShadow: "var(--shadow-card)",
            overflow: "hidden",
          }}
        >
          {/* Panel header */}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              padding: "12px 16px",
              borderBottom: "1px solid var(--color-separator)",
              background: "var(--color-surface)",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              {/* View toggle */}
              <div
                style={{
                  display: "flex",
                  borderRadius: "var(--radius-sm)",
                  border: "1px solid var(--color-separator)",
                  overflow: "hidden",
                }}
              >
                {(["structured", "raw"] as const).map((mode) => (
                  <button
                    key={mode}
                    onClick={() => setViewMode(mode)}
                    style={{
                      padding: "4px 12px",
                      border: "none",
                      background: viewMode === mode ? "var(--color-blue)" : "transparent",
                      color: viewMode === mode ? "white" : "var(--color-text-secondary)",
                      fontSize: 11,
                      fontWeight: 600,
                      cursor: "pointer",
                      display: "flex",
                      alignItems: "center",
                      gap: 4,
                    }}
                  >
                    {mode === "raw" && <Code size={10} />}
                    {mode === "structured" ? "Structured" : "Raw"}
                  </button>
                ))}
              </div>
              <span style={{ fontSize: 12, color: "var(--color-text-secondary)" }}>
                {activeEntry.description}
              </span>
            </div>
            <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
              {dirty && (
                <span style={{ fontSize: 11, color: "var(--color-amber)" }}>
                  unsaved changes
                </span>
              )}
              {dirty && (
                <button
                  onClick={handleReset}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 5,
                    padding: "5px 12px",
                    borderRadius: "var(--radius-md)",
                    border: "1px solid var(--color-separator)",
                    background: "var(--color-surface-2)",
                    color: "var(--color-text-secondary)",
                    fontSize: 12,
                    cursor: "pointer",
                  }}
                >
                  <RotateCcw size={11} /> Reset
                </button>
              )}
              <button
                onClick={handleSave}
                disabled={saving || !dirty}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 5,
                  padding: "5px 16px",
                  borderRadius: "var(--radius-md)",
                  border: "none",
                  background: dirty ? "var(--color-blue)" : "var(--color-surface-2)",
                  color: dirty ? "white" : "var(--color-text-tertiary)",
                  fontSize: 12,
                  fontWeight: 600,
                  cursor: dirty && !saving ? "pointer" : "default",
                  opacity: saving ? 0.7 : 1,
                }}
              >
                <Save size={11} />
                {saving ? "Saving…" : "Save"}
              </button>
            </div>
          </div>

          {/* Panel body */}
          {loading ? (
            <div
              style={{
                padding: 40,
                textAlign: "center",
                color: "var(--color-text-tertiary)",
                fontSize: 14,
              }}
            >
              Loading {activeEntry.label}…
            </div>
          ) : showStructured ? (
            <div style={{ padding: 16 }}>
              {activeConfig === "claude-code" && claudeSettings && (
                <ClaudeCodeEditor
                  settings={claudeSettings}
                  onChange={(s) => {
                    setClaudeSettings(s)
                    markDirty()
                  }}
                />
              )}
              {activeConfig === "codex" && codexConfig && (
                <CodexEditor
                  config={codexConfig}
                  onChange={(c) => {
                    setCodexConfig(c)
                    markDirty()
                  }}
                />
              )}
            </div>
          ) : (
            <textarea
              value={rawContent}
              onChange={(e) => {
                setRawContent(e.target.value)
                setDirty(e.target.value !== originalContent)
              }}
              spellCheck={false}
              style={{
                width: "100%",
                minHeight: 520,
                padding: 16,
                border: "none",
                outline: "none",
                resize: "vertical",
                background: "var(--color-bg-elevated)",
                color: "var(--color-text)",
                fontFamily: "var(--font-mono)",
                fontSize: 13,
                lineHeight: 1.65,
                boxSizing: "border-box",
              }}
            />
          )}
        </div>
      )}
    </div>
  )
}
