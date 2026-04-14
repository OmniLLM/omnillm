import {
  Moon,
  Sun,
  MessageSquare,
  BarChart3,
  Database,
  Settings,
  Layers,
} from "lucide-react"
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

import { createLogger } from "@/lib/logger"

const log = createLogger("app")

type Tab = "providers" | "chat" | "logging" | "vmodel" | "about"
type Theme = "dark" | "light"
type DesignSystem = "default" | "material"
type UXMode = "original" | "redesign"

const NAV_ITEMS = [
  { id: "providers" as const, label: "Providers", icon: Database },
  { id: "chat" as const, label: "Chat", icon: MessageSquare },
  { id: "logging" as const, label: "Logging", icon: BarChart3 },
  { id: "vmodel" as const, label: "Virtual Models", icon: Layers },
  { id: "about" as const, label: "About", icon: Settings },
]

const SIDEBAR_WIDTH = 260

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
  const { showToast } = useToast()
  const [info, setInfo] = useState<ServerInfo | null>(null)
  const [theme, setTheme] = useState<Theme>(loadTheme())
  const [designSystem, setDesignSystem] =
    useState<DesignSystem>(loadDesignSystem())
  const [uxMode, setUxMode] = useState<UXMode>(loadUXMode())

  useEffect(() => {
    log.info("initializing", { hostname: globalThis.location.hostname, port: globalThis.location.port })
    getInfo()
      .then((result) => {
        setInfo(result)
        log.debug("server info loaded", result)
      })
      .catch((err) => {
        log.error("failed to load server info", err)
      })
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

  const currentNavItem = NAV_ITEMS.find((n) => n.id === tab) ?? NAV_ITEMS[0]
  const Icon = currentNavItem.icon

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        background: "var(--color-bg)",
      }}
      data-design-system={designSystem}
    >
      {/* Sidebar */}
      <aside
        style={{
          width: SIDEBAR_WIDTH,
          flexShrink: 0,
          background: "var(--color-surface)",
          borderRight: "1px solid var(--color-separator)",
          display: "flex",
          flexDirection: "column",
          position: "fixed",
          top: 0,
          left: 0,
          bottom: 0,
          zIndex: 40,
        }}
      >
        {/* Sidebar Header: Logo */}
        <div
          style={{
            padding: "20px 20px 16px",
            borderBottom: "1px solid var(--color-separator)",
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div
              style={{
                width: 32,
                height: 32,
                borderRadius: "var(--radius-md)",
                background: "var(--color-blue)",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                flexShrink: 0,
                boxShadow: "0 2px 8px rgba(56,189,248,0.3)",
              }}
            >
              <svg width="16" height="16" viewBox="0 0 14 14" fill="none">
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
                  fontSize: 16,
                  color: "var(--color-text)",
                  letterSpacing: "-0.02em",
                  lineHeight: 1,
                }}
              >
                OmniModel
              </div>
              <div
                style={{
                  fontSize: 11,
                  color: "var(--color-text-tertiary)",
                  marginTop: 2,
                  fontFamily: "var(--font-text)",
                  letterSpacing: "-0.01em",
                }}
              >
                {designSystem === "material" ? "Material Design" : "Intelligent LLM Router"}
              </div>
            </div>
          </div>
        </div>

        {/* Nav Items */}
        <nav style={{ flex: 1, padding: "12px 10px" }}>
          <div
            style={{
              fontSize: 10,
              fontWeight: 600,
              textTransform: "uppercase",
              letterSpacing: "0.08em",
              color: "var(--color-text-tertiary)",
              padding: "8px 12px 6px",
            }}
          >
            Navigation
          </div>
          {NAV_ITEMS.map((item) => {
            const isActive = item.id === tab
            const ItemIcon = item.icon
            return (
              <button
                key={item.id}
                onClick={() => handleTabChange(item.id)}
                role="link"
                aria-label={`Navigate to ${item.label}`}
                aria-current={isActive ? "page" : undefined}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  width: "100%",
                  background: isActive ? "var(--color-blue-fill)" : "transparent",
                  border: "none",
                  color: isActive ? "var(--color-blue)" : "var(--color-text-secondary)",
                  fontFamily: "var(--font-text)",
                  fontSize: 13,
                  fontWeight: isActive ? 600 : 400,
                  letterSpacing: "-0.01em",
                  padding: "8px 12px",
                  borderRadius: "var(--radius-md)",
                  cursor: "pointer",
                  transition: "all 0.15s var(--ease)",
                  textAlign: "left",
                }}
              >
                <ItemIcon size={16} style={{ flexShrink: 0 }} />
                {item.label}
              </button>
            )
          })}
        </nav>

        {/* Sidebar Footer: Status + Theme */}
        <div
          style={{
            padding: "16px 16px",
            borderTop: "1px solid var(--color-separator)",
          }}
        >
          {/* Status */}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              marginBottom: 12,
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
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
            {info && (
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 10,
                  color: "var(--color-text-tertiary)",
                }}
              >
                v{info.version} · :{info.port}
              </span>
            )}
          </div>

          {/* Theme Toggle */}
          <button
            onClick={toggleTheme}
            title={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            aria-label={theme === "dark" ? "Switch to light theme" : "Switch to dark theme"}
            style={{
              width: "100%",
              height: 36,
              borderRadius: "var(--radius-md)",
              border: "1px solid var(--color-separator)",
              background: "var(--color-surface-2)",
              color: "var(--color-text-secondary)",
              cursor: "pointer",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              gap: 8,
              fontSize: 12,
              fontWeight: 500,
              transition: "all 0.15s var(--ease)",
            }}
          >
            {theme === "dark" ? <Sun size={14} /> : <Moon size={14} />}
            {theme === "dark" ? "Light" : "Dark"} mode
          </button>

          {/* Hidden dev toggles */}
          <div style={{ display: "flex", gap: 6, marginTop: 8 }}>
            {false && (
              <button
                onClick={toggleUXMode}
                title={uxMode === "original" ? "Switch to Control Tower UX" : "Switch to Original UX"}
                style={{
                  flex: 1,
                  height: 28,
                  borderRadius: "var(--radius-sm)",
                  border: "1px solid var(--color-separator)",
                  background: "var(--color-surface-2)",
                  color: "var(--color-text-tertiary)",
                  cursor: "pointer",
                  fontSize: 10,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                {uxMode === "original" ? "Original" : "Redesign"}
              </button>
            )}
            {false && (
              <button
                onClick={toggleDesignSystem}
                title={designSystem === "default" ? "Switch to Material Design" : "Switch to Default Design"}
                style={{
                  flex: 1,
                  height: 28,
                  borderRadius: "var(--radius-sm)",
                  border: "1px solid var(--color-separator)",
                  background: "var(--color-surface-2)",
                  color: "var(--color-text-tertiary)",
                  cursor: "pointer",
                  fontSize: 10,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                }}
              >
                {designSystem === "default" ? "Default" : "Material"}
              </button>
            )}
          </div>
        </div>
      </aside>

      {/* Main Content Area */}
      <div
        style={{
          flex: 1,
          marginLeft: SIDEBAR_WIDTH,
          display: "flex",
          flexDirection: "column",
          minHeight: "100vh",
        }}
      >
        {/* Top Bar */}
        <header
          style={{
            background: "var(--color-bg)",
            backdropFilter: "blur(20px) saturate(180%)",
            WebkitBackdropFilter: "blur(20px) saturate(180%)",
            borderBottom: "1px solid var(--color-separator)",
            padding: "0 32px",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            height: 56,
            flexShrink: 0,
            position: "sticky",
            top: 0,
            zIndex: 30,
          }}
        >
          <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <Icon size={18} style={{ color: "var(--color-blue)", flexShrink: 0 }} />
            <h1
              style={{
                fontSize: 15,
                fontWeight: 600,
                color: "var(--color-text)",
                margin: 0,
                fontFamily: "var(--font-display)",
                letterSpacing: "-0.02em",
              }}
            >
              {currentNavItem.label}
            </h1>
          </div>
        </header>

        {/* Page Content */}
        <MuiThemeWrapper isDark={theme === "dark"}>
          <main
            style={{
              flex: 1,
              padding: designSystem === "material" ? "0" : (tab === "chat" ? "0" : "32px 32px 40px"),
              maxWidth: designSystem === "material" ? "none" : 1440,
              width: "100%",
              margin: "0 auto",
            }}
          >
            <div
              className="animate-slide-in"
              style={{
                padding: designSystem === "material" ? "32px 24px 40px" : (tab === "chat" ? "0" : "0"),
                minHeight:
                  designSystem === "material" ? "calc(100dvh - 56px)" : (tab === "chat" ? "calc(100dvh - 56px)" : "auto"),
              }}
            >
              <div
                style={{
                  maxWidth: designSystem === "material" ? 1280 : 1200,
                  margin: "0 auto",
                  width: "100%",
                }}
              >
                {designSystem === "material" ?
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
      </div>

      <ToastContainer />
    </div>
  )
}
