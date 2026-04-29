import {
  Moon,
  Sun,
  MessageSquare,
  BarChart3,
  Database,
  Settings,
  Layers,
  ChevronLeft,
  ChevronRight,
  SlidersHorizontal,
  Menu,
} from "lucide-react"
import { useState, useEffect, useMemo } from "react"
import { useTranslation } from "react-i18next"

import { getInfo, type ServerInfo } from "@/api"
import { MuiThemeWrapper } from "@/components/MuiThemeWrapper"
import { useToast, ToastContainer } from "@/components/Toast"
import { useLanguage } from "@/hooks/useLanguage"
import { createLogger } from "@/lib/logger"
import { ConfirmProvider } from "@/lib/useConfirm"
import { useMediaQuery } from "@/lib/useMediaQuery"
import { ChatPage } from "@/pages/ChatPage"
import { ConfigPage } from "@/pages/ConfigPage"
import { LoggingPage } from "@/pages/LoggingPage"
import { ProvidersPage } from "@/pages/ProvidersPage"
import { AboutPage } from "@/pages/SettingsPage"
import { VirtualModelPage } from "@/pages/VirtualModelPage"

const log = createLogger("app")

type Tab =
  | "providers"
  | "chat"
  | "logging"
  | "virtualmodel"
  | "config"
  | "about"
type Theme = "dark" | "light"

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
    || value === "virtualmodel"
    || value === "config"
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

export default function AppComponent() {
  const { t } = useTranslation(["nav", "common"])
  const { toggleLanguage, currentLanguage } = useLanguage()
  const [tab, setTab] = useState<Tab>(loadTab())
  const { showToast } = useToast()
  const [info, setInfo] = useState<ServerInfo | null>(null)
  const [theme, setTheme] = useState<Theme>(loadTheme())
  const [sidebarCollapsed, setSidebarCollapsed] = useState(loadSidebarCollapsed)
  const [toggleHovered, setToggleHovered] = useState(false)
  const isMobile = useMediaQuery("(max-width: 768px)")
  const [mobileOpen, setMobileOpen] = useState(false)

  // Create NAV_ITEMS with translations
  const NAV_ITEMS = useMemo(
    () => [
      { id: "providers" as const, label: t("nav:providers"), icon: Database },
      { id: "chat" as const, label: t("nav:chat"), icon: MessageSquare },
      { id: "logging" as const, label: t("nav:logging"), icon: BarChart3 },
      {
        id: "virtualmodel" as const,
        label: t("nav:virtualModels"),
        icon: Layers,
      },
      {
        id: "config" as const,
        label: t("nav:toolConfig"),
        icon: SlidersHorizontal,
      },
      { id: "about" as const, label: t("nav:about"), icon: Settings },
    ],
    [t],
  )

  // Auto-collapse mobile drawer on tab change
  useEffect(() => {
    if (isMobile) setMobileOpen(false)
  }, [tab, isMobile])

  // ESC closes mobile drawer
  useEffect(() => {
    if (!mobileOpen) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMobileOpen(false)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [mobileOpen])

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

  // On mobile, the sidebar is an overlay drawer driven by `mobileOpen`.
  // On desktop, it follows the persistent `sidebarCollapsed` preference.
  const effectiveCollapsed = isMobile ? !mobileOpen : sidebarCollapsed
  const sidebarOverlay = isMobile && mobileOpen

  return (
    <ConfirmProvider>
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
            width: effectiveCollapsed ? 0 : SIDEBAR_WIDTH,
            minWidth: effectiveCollapsed ? 0 : SIDEBAR_WIDTH,
            flexShrink: 0,
            background:
              effectiveCollapsed ? "transparent" : "var(--color-surface)",
            borderRight:
              effectiveCollapsed ? "none" : "1px solid var(--color-separator)",
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
          {!effectiveCollapsed && (
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
                      {t("common:headers.omnillm")}
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
                      {t("common:headers.intelligentLLMRouter")}
                    </div>
                  </div>
                </div>
              </div>

              <nav
                aria-label={t("common:headers.mainNavigation")}
                style={{ flex: 1, padding: "10px 0 10px 0" }}
              >
                {NAV_ITEMS.map((item) => {
                  const isActive = item.id === tab
                  const ItemIcon = item.icon
                  return (
                    <button
                      key={item.id}
                      onClick={() => handleTabChange(item.id)}
                      aria-current={isActive ? "page" : undefined}
                      className={`nav-item${isActive ? " active" : ""}`}
                    >
                      <ItemIcon size={15} style={{ flexShrink: 0 }} />
                      {item.label}
                    </button>
                  )
                })}
              </nav>

              <div
                style={{
                  padding: "14px 16px",
                  borderTop: "1px solid var(--color-separator)",
                }}
              >
                <div
                  style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                  }}
                >
                  <div
                    style={{ display: "flex", alignItems: "center", gap: 8 }}
                  >
                    <div
                      className="status-dot status-dot-active"
                      role="img"
                      aria-label={t("common:sidebar.serverOnlineAriaLabel")}
                    />
                    <span className="sr-only">
                      {t("common:sidebar.statusDot")}.
                    </span>
                    {info ?
                      <span className="version-pill">
                        v{info.version} · :{info.port}
                      </span>
                    : <span
                        style={{
                          color: "var(--color-green)",
                          fontSize: 12,
                          fontWeight: 500,
                        }}
                      >
                        {t("common:common.online")}
                      </span>
                    }
                  </div>
                  <div
                    style={{ display: "flex", alignItems: "center", gap: 4 }}
                  >
                    <button
                      onClick={toggleLanguage}
                      title={
                        currentLanguage() === "en" ?
                          t("common:language.switchToChinese")
                        : t("common:language.switchToEnglish")
                      }
                      aria-label={
                        currentLanguage() === "en" ?
                          t("common:language.switchToChinese")
                        : t("common:language.switchToEnglish")
                      }
                      className="btn btn-icon btn-icon-ghost"
                      style={{ fontSize: 11, fontWeight: 600 }}
                    >
                      {currentLanguage() === "en" ? "中文" : "EN"}
                    </button>
                    <button
                      onClick={toggleTheme}
                      title={`${t("common:theme.switchTo")}${theme === "dark" ? t("common:theme.switchToLight") : t("common:theme.switchToDark")}`}
                      aria-label={`${t("common:theme.switchTo")}${theme === "dark" ? t("common:theme.switchToLight") : t("common:theme.switchToDark")}`}
                      className="btn btn-icon btn-icon-ghost"
                    >
                      {theme === "dark" ?
                        <Sun size={14} />
                      : <Moon size={14} />}
                    </button>
                  </div>
                </div>
              </div>
            </>
          )}
        </aside>

        {sidebarOverlay && (
          <div
            onClick={() => setMobileOpen(false)}
            aria-hidden="true"
            style={{
              position: "fixed",
              inset: 0,
              background: "rgba(1,4,9,0.5)",
              backdropFilter: "blur(2px)",
              zIndex: 39,
            }}
          />
        )}

        {!isMobile && (
          <button
            onClick={toggleSidebar}
            onMouseEnter={() => setToggleHovered(true)}
            onMouseLeave={() => setToggleHovered(false)}
            title={
              sidebarCollapsed ?
                t("common:sidebar.showSidebar")
              : t("common:sidebar.hideSidebar")
            }
            aria-label={
              sidebarCollapsed ?
                t("common:sidebar.showSidebar")
              : t("common:sidebar.hideSidebar")
            }
            style={{
              position: "fixed",
              top: "50%",
              left: sidebarCollapsed ? 0 : SIDEBAR_WIDTH - 12,
              transform: "translateY(-50%)",
              width: 24,
              height: 72,
              borderRadius: "0 8px 8px 0",
              border: "1px solid var(--color-separator)",
              borderLeft: "none",
              background:
                toggleHovered ? "var(--color-blue)" : "var(--color-surface)",
              color: toggleHovered ? "white" : "var(--color-text-tertiary)",
              boxShadow:
                toggleHovered ? "0 2px 8px rgba(56,189,248,0.3)" : "none",
              cursor: "pointer",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              transition:
                "left 0.2s ease, background 0.15s ease, color 0.15s ease",
              zIndex: 41,
              padding: 0,
            }}
          >
            {sidebarCollapsed ?
              <ChevronRight size={14} strokeWidth={2.5} />
            : <ChevronLeft size={14} strokeWidth={2.5} />}
          </button>
        )}

        <div
          className="app-main"
          style={{
            flex: 1,
            marginLeft: isMobile || sidebarCollapsed ? 0 : SIDEBAR_WIDTH,
            display: "flex",
            flexDirection: "column",
            minHeight: "100vh",
            transition: "margin-left 0.2s ease",
            minWidth: 0,
          }}
        >
          <header
            style={{
              background: "var(--color-header-bg)",
              backdropFilter: "blur(20px) saturate(180%)",
              WebkitBackdropFilter: "blur(20px) saturate(180%)",
              borderBottom: "1px solid var(--color-separator)",
              padding: "0 28px",
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              height: 52,
              flexShrink: 0,
              position: "sticky",
              top: 0,
              zIndex: 30,
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              {isMobile && (
                <button
                  onClick={() => setMobileOpen((v) => !v)}
                  className="btn btn-icon btn-icon-ghost"
                  aria-label={
                    mobileOpen ?
                      t("common:sidebar.close")
                    : t("common:sidebar.open")
                  }
                  aria-expanded={mobileOpen}
                  style={{ marginRight: 4 }}
                >
                  <Menu size={16} />
                </button>
              )}
              <span
                style={{
                  fontSize: 12,
                  color: "var(--color-text-tertiary)",
                  fontFamily: "var(--font-text)",
                }}
              >
                {t("common:headers.omnillm")}
              </span>
              <span
                style={{ fontSize: 12, color: "var(--color-separator-opaque)" }}
              >
                /
              </span>
              <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
                <Icon
                  size={14}
                  style={{ color: "var(--color-blue)", flexShrink: 0 }}
                />
                <h1
                  style={{
                    fontSize: 13,
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
            </div>
            {info && (
              <div
                style={{ display: "flex", alignItems: "center", gap: 6 }}
                title={t("common:sidebar.serverOnlineAriaLabel")}
              >
                <div
                  className="status-dot status-dot-active"
                  role="img"
                  aria-label={t("common:sidebar.serverOnlineAriaLabel")}
                />
                <span
                  style={{
                    fontSize: 11,
                    color: "var(--color-text-tertiary)",
                    fontFamily: "var(--font-mono)",
                  }}
                >
                  :{info.port}
                </span>
              </div>
            )}
          </header>

          <MuiThemeWrapper isDark={theme === "dark"}>
            <main
              style={{
                flex: 1,
                padding: tab === "chat" ? "0" : "28px 28px 40px",
                maxWidth: 1440,
                width: "100%",
                margin: "0 auto",
              }}
            >
              <div
                className="animate-slide-in"
                style={{
                  padding: tab === "chat" ? "0" : "0",
                  minHeight: tab === "chat" ? "calc(100dvh - 52px)" : "auto",
                }}
              >
                <div
                  style={{
                    maxWidth: 1200,
                    margin: "0 auto",
                    width: "100%",
                  }}
                >
                  {tab === "providers" && (
                    <ProvidersPage showToast={showToast} />
                  )}
                  {tab === "chat" && <ChatPage showToast={showToast} />}
                  {tab === "logging" && <LoggingPage showToast={showToast} />}
                  {tab === "virtualmodel" && (
                    <VirtualModelPage showToast={showToast} />
                  )}
                  {tab === "config" && <ConfigPage showToast={showToast} />}
                  {tab === "about" && <AboutPage showToast={showToast} />}
                </div>
              </div>
            </main>
          </MuiThemeWrapper>
        </div>

        <ToastContainer />
      </div>
    </ConfirmProvider>
  )
}
