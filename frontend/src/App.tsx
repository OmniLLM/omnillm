import {
  Moon,
  Sun,
  MessageSquare,
  BarChart3,
  Database,
  Settings,
  Layers,
  PanelLeftOpen,
  PanelLeftClose,
} from "lucide-react"
import { useState, useEffect } from "react"

import { getInfo, type ServerInfo } from "@/api"
import { MuiThemeWrapper } from "@/components/MuiThemeWrapper"
import { useToast, ToastContainer } from "@/components/Toast"
import { createLogger } from "@/lib/logger"
import { ChatPage } from "@/pages/ChatPage"
import { LoggingPage } from "@/pages/LoggingPage"
import { ProvidersPage } from "@/pages/ProvidersPage"
import { AboutPage } from "@/pages/SettingsPage"
import { VmodelPage } from "@/pages/VmodelPage"

const log = createLogger("app")

type Tab = "providers" | "chat" | "logging" | "vmodel" | "about"
type Theme = "dark" | "light"

const NAV_ITEMS = [
  { id: "providers" as const, label: "Providers", icon: Database },
  { id: "chat" as const, label: "Chat", icon: MessageSquare },
  { id: "logging" as const, label: "Logging", icon: BarChart3 },
  { id: "vmodel" as const, label: "Virtual Models", icon: Layers },
  { id: "about" as const, label: "About", icon: Settings },
]

const SIDEBAR_WIDTH = 260

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
    globalThis.location.hash = tab
  } catch {
    // ignore
  }
}

function loadSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem("olp-sidebar-collapsed") === "true"
  } catch {
    return false
  }
}

function applyTheme(theme: Theme) {
  if (theme === "light") document.documentElement.dataset.theme = "light"
  else delete document.documentElement.dataset.theme
}

applyTheme(loadTheme())

// eslint-disable-next-line max-lines-per-function, complexity
export default function AppComponent() {
  const [tab, setTab] = useState<Tab>(loadTab())
  const { showToast } = useToast()
  const [info, setInfo] = useState<ServerInfo | null>(null)
  const [theme, setTheme] = useState<Theme>(loadTheme())
  const [sidebarCollapsed, setSidebarCollapsed] = useState(loadSidebarCollapsed)

  useEffect(() => {
    log.info("initializing", {
      hostname: globalThis.location.hostname,
      port: globalThis.location.port,
    })
    getInfo()
      .then((result) => {
        setInfo(result)
        log.debug("server info loaded", result)
      })
      .catch((err: unknown) => {
        log.error("failed to load server info", err)
      })
  }, [])

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

  const toggleSidebar = () => {
    const next = !sidebarCollapsed
    setSidebarCollapsed(next)
    try {
      localStorage.setItem("olp-sidebar-collapsed", String(next))
    } catch {
      /* ignore */
    }
  }

  const currentNavItem = NAV_ITEMS.find((n) => n.id === tab) ?? NAV_ITEMS[0]
  const Icon = currentNavItem.icon

  return (
    <div
      className="app-shell"
      style={{
        minHeight: "100vh",
        display: "flex",
        background: "var(--color-bg)",
      }}
    >
      <aside
        className="app-sidebar"
        style={{
          width: sidebarCollapsed ? 0 : SIDEBAR_WIDTH,
          minWidth: sidebarCollapsed ? 0 : SIDEBAR_WIDTH,
          flexShrink: 0,
          background: sidebarCollapsed ? "transparent" : "var(--color-surface)",
          borderRight:
            sidebarCollapsed ? "none" : "1px solid var(--color-separator)",
          display: "flex",
          flexDirection: "column",
          position: "fixed",
          top: 0,
          left: 0,
          bottom: 0,
          zIndex: 40,
          overflow: "hidden",
          transition: "width 0.2s ease, min-width 0.2s ease",
        }}
      >
        {!sidebarCollapsed && (
          <>
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
                    Intelligent LLM Router
                  </div>
                </div>
              </div>
            </div>

            <nav
              aria-label="Main navigation"
              style={{ flex: 1, padding: "12px 10px" }}
            >
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
                    aria-current={isActive ? "page" : undefined}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 10,
                      width: "100%",
                      background:
                        isActive ? "var(--color-blue-fill)" : "transparent",
                      border: "none",
                      color:
                        isActive ? "var(--color-blue)" : (
                          "var(--color-text-secondary)"
                        ),
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

            <div
              style={{
                padding: "16px 16px",
                borderTop: "1px solid var(--color-separator)",
              }}
            >
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

              <button
                onClick={toggleTheme}
                title={
                  theme === "dark" ?
                    "Switch to light theme"
                  : "Switch to dark theme"
                }
                aria-label={
                  theme === "dark" ?
                    "Switch to light theme"
                  : "Switch to dark theme"
                }
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
                {theme === "dark" ?
                  <Sun size={14} />
                : <Moon size={14} />}
                Switch to {theme === "dark" ? "light" : "dark"}
              </button>
            </div>
          </>
        )}
      </aside>

      <div
        className="app-main"
        style={{
          flex: 1,
          marginLeft: sidebarCollapsed ? 0 : SIDEBAR_WIDTH,
          display: "flex",
          flexDirection: "column",
          minHeight: "100vh",
          transition: "margin-left 0.2s ease",
        }}
      >
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
            <button
              onClick={toggleSidebar}
              title={sidebarCollapsed ? "Show sidebar" : "Hide sidebar"}
              aria-label={sidebarCollapsed ? "Show sidebar" : "Hide sidebar"}
              style={{
                width: 36,
                height: 36,
                borderRadius: "var(--radius-md)",
                border: "none",
                background: "transparent",
                color: "var(--color-text-secondary)",
                cursor: "pointer",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                transition: "all 0.15s var(--ease)",
              }}
            >
              {sidebarCollapsed ?
                <PanelLeftOpen size={18} />
              : <PanelLeftClose size={18} />}
            </button>
            <Icon
              size={18}
              style={{ color: "var(--color-blue)", flexShrink: 0 }}
            />
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

        <MuiThemeWrapper isDark={theme === "dark"}>
          <main
            style={{
              flex: 1,
              padding: tab === "chat" ? "0" : "32px 32px 40px",
              maxWidth: 1440,
              width: "100%",
              margin: "0 auto",
            }}
          >
            <div
              className="animate-slide-in"
              style={{
                padding: tab === "chat" ? "0" : "0",
                minHeight: tab === "chat" ? "calc(100dvh - 56px)" : "auto",
              }}
            >
              <div
                style={{
                  maxWidth: 1200,
                  margin: "0 auto",
                  width: "100%",
                }}
              >
                {tab === "providers" && <ProvidersPage showToast={showToast} />}
                {tab === "chat" && <ChatPage showToast={showToast} />}
                {tab === "logging" && <LoggingPage showToast={showToast} />}
                {tab === "vmodel" && <VmodelPage showToast={showToast} />}
                {tab === "about" && <AboutPage showToast={showToast} />}
              </div>
            </div>
          </main>
        </MuiThemeWrapper>
      </div>

      <ToastContainer />
    </div>
  )
}
