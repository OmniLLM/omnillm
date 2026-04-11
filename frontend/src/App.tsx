import { Moon, Sun } from "lucide-react"
import { useState, useEffect } from "react"

import { getInfo, type ServerInfo } from "@/api"
import { MuiThemeWrapper } from "@/components/MuiThemeWrapper"
import { useToast, ToastContainer } from "@/components/Toast"
import { ChatPage } from "@/pages/ChatPage"
import { LoggingPage } from "@/pages/LoggingPage"
import { MaterialLoggingPageComplete } from "@/pages/MaterialLoggingPageComplete"
import { MaterialProvidersPageComplete } from "@/pages/MaterialProvidersPageComplete"
import { MaterialAboutPageComplete } from "@/pages/MaterialSettingsPageComplete"
import { ProvidersPage } from "@/pages/ProvidersPage"
import { ProvidersPageRedesign } from "@/pages/ProvidersPageRedesign"
import { AboutPage } from "@/pages/SettingsPage"
import { VmodelPage } from "@/pages/VmodelPage"
// Import Material Design overlay styles
import "@/styles/material-overlay.css"

type Tab = "providers" | "chat" | "logging" | "vmodel" | "about"
type Theme = "dark" | "light"
type DesignSystem = "default" | "material"
type UXMode = "original" | "redesign"

function loadUXMode(): UXMode {
  try {
    const stored = localStorage.getItem("olp-ux-mode")
    return stored === "redesign" ? "redesign" : "original"
  } catch {
    return "original"
  }
}

function loadDesignSystem(): DesignSystem {
  try {
    const stored = localStorage.getItem("olp-design-system")
    return stored === "material" ? "material" : "default"
  } catch {
    return "default"
  }
}

function loadTheme(): Theme {
  try {
    const stored = localStorage.getItem("olp-theme")
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
    return (stored as Theme) || "dark"
  } catch {
    return "dark"
  }
}

function isTab(value: string): value is Tab {
  return (
    value === "providers"
    || value === "chat"
    || value === "logging"
    || value === "vmodel"
    || value === "about"
  )
}

function loadTab(): Tab {
  try {
    const hash = globalThis.location.hash.slice(1)
    if (isTab(hash)) {
      return hash
    }

    const stored = localStorage.getItem("olp-current-tab")
    if (stored && isTab(stored)) {
      return stored
    }

    return "providers"
  } catch {
    return "providers"
  }
}

function saveTab(tab: Tab) {
  try {
    localStorage.setItem("olp-current-tab", tab)
    // Also update URL hash for immediate refresh support
    globalThis.location.hash = tab
  } catch {
    // ignore
  }
}

function applyTheme(theme: Theme) {
  if (theme === "light") document.documentElement.dataset.theme = "light"
  else delete document.documentElement.dataset.theme
}

// Apply before first render
applyTheme(loadTheme())

// eslint-disable-next-line max-lines-per-function
export default function AppComponent() {
  const [tab, setTab] = useState<Tab>(loadTab())
  const { toasts, showToast } = useToast()
  const [info, setInfo] = useState<ServerInfo | null>(null)
  const [theme, setTheme] = useState<Theme>(loadTheme())
  const [designSystem, setDesignSystem] =
    useState<DesignSystem>(loadDesignSystem())
  const [uxMode, setUxMode] = useState<UXMode>(loadUXMode())

  useEffect(() => {
    getInfo()
      .then(setInfo)
      .catch(() => {})
  }, [])

  // Handle browser back/forward navigation
  useEffect(() => {
    const handleHashChange = () => {
      const newTab = loadTab()
      setTab(newTab)
    }

    globalThis.addEventListener("hashchange", handleHashChange)
    return () => globalThis.removeEventListener("hashchange", handleHashChange)
  }, [])

  const handleTabChange = (newTab: Tab) => {
    setTab(newTab)
    saveTab(newTab)
  }

  const toggleUXMode = () => {
    const next: UXMode = uxMode === "original" ? "redesign" : "original"
    setUxMode(next)
    try {
      localStorage.setItem("olp-ux-mode", next)
    } catch {
      /* ignore */
    }
  }

  const toggleDesignSystem = () => {
    const next: DesignSystem =
      designSystem === "default" ? "material" : "default"
    setDesignSystem(next)
    try {
      localStorage.setItem("olp-design-system", next)
    } catch {
      /* ignore */
    }
  }

  const toggleTheme = () => {
    const next: Theme = theme === "dark" ? "light" : "dark"
    setTheme(next)
    applyTheme(next)
    try {
      localStorage.setItem("olp-theme", next)
    } catch {
      /* ignore */
    }
  }

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        flexDirection: "column",
        background: "var(--color-bg)",
      }}
      data-design-system={designSystem}
    >
      {/* Header */}
      <header
        style={{
          background: "var(--color-header-bg)",
          backdropFilter: "blur(20px) saturate(180%)",
          WebkitBackdropFilter: "blur(20px) saturate(180%)",
          borderBottom: "1px solid var(--color-separator)",
          padding: "0 24px",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          height: 64,
          flexShrink: 0,
          position: "sticky",
          top: 0,
          zIndex: 50,
        }}
      >
        {/* Left: logo + title */}
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <div
            style={{
              width: 28,
              height: 28,
              borderRadius: "var(--radius-sm)",
              background: "var(--color-blue)",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              flexShrink: 0,
              boxShadow: "0 1px 4px rgba(10,132,255,0.4)",
            }}
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
              <path
                d="M7 1L12.196 4V10L7 13L1.804 10V4L7 1Z"
                stroke="white"
                strokeWidth="1.5"
                fill="none"
                opacity="0.9"
              />
              <circle cx="7" cy="7" r="2.5" fill="white" />
            </svg>
          </div>
          <div>
            <div
              style={{
                fontFamily: "var(--font-display)",
                fontWeight: 700,
                fontSize: 15,
                color: "var(--color-text)",
                letterSpacing: "-0.02em",
                lineHeight: 1,
              }}
            >
              LLM Proxy
            </div>
            <div
              style={{
                fontSize: 11,
                color: "var(--color-text-tertiary)",
                marginTop: 1,
                fontFamily: "var(--font-text)",
                letterSpacing: "-0.01em",
              }}
            >
              {designSystem === "material" ?
                "Material Design"
              : "Model Routing"}
            </div>
          </div>
        </div>

        {/* Center: segmented nav */}
        <nav
          style={{
            display: "flex",
            background: "rgba(255,255,255,0.06)",
            borderRadius: "var(--radius-lg)",
            padding: 3,
            gap: 2,
          }}
        >
          {(["providers", "chat", "logging", "vmodel", "about"] as Array<Tab>).map((t) => (
            <button
              key={t}
              onClick={() => handleTabChange(t)}
              style={{
                background: tab === t ? "var(--color-surface)" : "transparent",
                border: "none",
                color:
                  tab === t ? "var(--color-text)" : (
                    "var(--color-text-secondary)"
                  ),
                fontFamily: "var(--font-text)",
                fontSize: 13,
                fontWeight: tab === t ? 600 : 400,
                letterSpacing: "-0.01em",
                padding: "5px 16px",
                borderRadius: "var(--radius-md)",
                cursor: "pointer",
                transition: "all 0.15s var(--ease)",
                boxShadow: tab === t ? "0 1px 3px rgba(0,0,0,0.3)" : "none",
              }}
            >
              {t === "providers" && "Providers"}
              {t === "chat" && "Chat"}
              {t === "logging" && "Logging"}
              {t === "vmodel" && "Vmodel"}
              {t === "about" && "About"}
            </button>
          ))}
        </nav>

        {/* Right: info + status */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            fontSize: 12,
            color: "var(--color-text-tertiary)",
          }}
        >
          <button
            onClick={toggleTheme}
            title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            style={{
              height: 36,
              padding: "0 12px",
              borderRadius: "9999px",
              border: "1px solid var(--color-separator)",
              background: "var(--color-surface)",
              color: "var(--color-text)",
              cursor: "pointer",
              display: "inline-flex",
              alignItems: "center",
              gap: 8,
              fontSize: 12,
              fontWeight: 600,
              transition: "all 0.15s var(--ease)",
              boxShadow: "var(--shadow-btn)",
              flexShrink: 0,
            }}
            onMouseEnter={(e) => {
              ;(e.currentTarget as HTMLButtonElement).style.background =
                "var(--color-surface-2)"
            }}
            onMouseLeave={(e) => {
              ;(e.currentTarget as HTMLButtonElement).style.background =
                "var(--color-surface)"
            }}
          >
            {theme === "dark" ? <Sun size={14} /> : <Moon size={14} />}
            <span>{theme === "dark" ? "Light" : "Dark"}</span>
          </button>
          <div
            style={{
              width: 1,
              height: 24,
              background: "var(--color-separator)",
              flexShrink: 0,
            }}
          />
          {info && (
            <span
              style={{
                fontFamily: "var(--font-mono)",
                fontSize: 11,
                color: "var(--color-text-secondary)",
              }}
            >
              v{info.version} · :{info.port}
            </span>
          )}
          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <div className="status-dot status-dot-active" />
            <span
              style={{
                color: "var(--color-green)",
                fontSize: 12,
                fontWeight: 500,
              }}
            >
              Online
            </span>
          </div>
          {/* UX Mode Toggle - Hidden */}
          {false && (
          <button
            onClick={toggleUXMode}
            title={
              uxMode === "original" ?
                "Switch to Control Tower UX"
              : "Switch to Original UX"
            }
            style={{
              width: 32,
              height: 32,
              borderRadius: "var(--radius-sm)",
              border: "1px solid var(--color-separator)",
              background: "var(--color-surface)",
              color: "var(--color-text-secondary)",
              cursor: "pointer",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 15,
              transition: "all 0.15s var(--ease)",
              flexShrink: 0,
            }}
            onMouseEnter={(e) => {
              ;(e.currentTarget as HTMLButtonElement).style.background =
                "var(--color-surface-2)"
            }}
            onMouseLeave={(e) => {
              ;(e.currentTarget as HTMLButtonElement).style.background =
                "var(--color-surface)"
            }}
          >
            {uxMode === "original" ? "✨" : "📋"}
          </button>
          )}
          {/* Design System toggle - Hidden */}
          {false && (
          <button
            onClick={toggleDesignSystem}
            title={
              designSystem === "default" ?
                "Switch to Material Design"
              : "Switch to Default Design"
            }
            style={{
              width: 32,
              height: 32,
              borderRadius: "var(--radius-sm)",
              border: "1px solid var(--color-separator)",
              background: "var(--color-surface)",
              color: "var(--color-text-secondary)",
              cursor: "pointer",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              fontSize: 15,
              transition: "all 0.15s var(--ease)",
              flexShrink: 0,
            }}
            onMouseEnter={(e) => {
              ;(e.currentTarget as HTMLButtonElement).style.background =
                "var(--color-surface-2)"
            }}
            onMouseLeave={(e) => {
              ;(e.currentTarget as HTMLButtonElement).style.background =
                "var(--color-surface)"
            }}
          >
            {designSystem === "default" ? "🎨" : "🏠"}
          </button>
          )}
        </div>
      </header>

      {/* Content */}
      <MuiThemeWrapper isDark={theme === "dark"}>
        <main
          style={{
            flex: 1,
            padding: designSystem === "material" ? "0" : (tab === "chat" ? "0" : "32px 24px 40px"),
            maxWidth: designSystem === "material" ? "none" : 1280,
            width: "100%",
            margin: "0 auto",
          }}
        >
          <div
            className="animate-slide-in"
            style={{
              padding: designSystem === "material" ? "32px 24px 40px" : (tab === "chat" ? "0" : "0"),
              minHeight:
                designSystem === "material" ? "calc(100vh - 64px)" : (tab === "chat" ? "calc(100vh - 64px)" : "auto"),
            }}
          >
            <div
              style={{
                maxWidth: designSystem === "material" ? 1280 : 1200,
                margin: "0 auto",
                width: "100%",
              }}
            >
              {
              designSystem === "material" ?
                // Use Material Design pages
                <>
                  {tab === "providers" && <MaterialProvidersPageComplete showToast={showToast} />}
                  {tab === "chat" && <ChatPage showToast={showToast} />}
                  {tab === "logging" && <MaterialLoggingPageComplete showToast={showToast} />}
                  {tab === "vmodel" && <VmodelPage showToast={showToast} />}
                  {tab === "about" && <MaterialAboutPageComplete showToast={showToast} />}
                </>
                // Default versions
              : <>
                  {tab === "providers"
                    && (uxMode === "redesign" ?
                      <ProvidersPageRedesign showToast={showToast} />
                    : <ProvidersPage showToast={showToast} />)}
                  {tab === "chat" && <ChatPage showToast={showToast} />}
                  {tab === "logging" && <LoggingPage showToast={showToast} />}
                  {tab === "vmodel" && <VmodelPage showToast={showToast} />}
                  {tab === "about" && <AboutPage showToast={showToast} />}
                </>

            }
            </div>
          </div>
        </main>
      </MuiThemeWrapper>

      <ToastContainer toasts={toasts} />
    </div>
  )
}
