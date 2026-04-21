import {
  Fragment,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react"

import {
  getLogLevel,
  subscribeToLogs,
  updateLogLevel,
  type LogLevel,
} from "@/api"
import { createLogger } from "@/lib/logger"
import { parseLogLine } from "@/lib/logs"

const log = createLogger("logging-page")

const LOG_LEVELS = [
  { value: "trace", label: "Trace" },
  { value: "debug", label: "Debug" },
  { value: "info", label: "Info" },
  { value: "warn", label: "Warn" },
  { value: "error", label: "Error" },
  { value: "fatal", label: "Fatal" },
] as const

const MAX_LINES = 500

function getLogToneStyles(level: number): {
  accent: string
  background: string
} {
  switch (level) {
    case 0:
    case 1: {
      return {
        accent: "var(--color-red)",
        background: "var(--color-red-fill)",
      }
    }
    case 2: {
      return {
        accent: "var(--color-orange)",
        background: "var(--color-orange-fill)",
      }
    }
    case 3: {
      return {
        accent: "var(--color-blue)",
        background: "var(--color-blue-fill)",
      }
    }
    case 4: {
      return {
        accent: "var(--color-green)",
        background: "var(--color-green-fill)",
      }
    }
    default: {
      return {
        accent: "var(--color-text-secondary)",
        background: "rgba(148, 163, 184, 0.08)",
      }
    }
  }
}

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

export function LoggingPage({
  showToast,
}: {
  showToast: (msg: string, type?: "success" | "error") => void
}) {
  const [lines, setLines] = useState<Array<string>>([])
  const [connected, setConnected] = useState(false)
  const [connecting, setConnecting] = useState(true)
  const [autoScroll, setAutoScroll] = useState(true)
  const [logLevel, setLogLevelState] = useState<LogLevel | null>(null)
  const logViewportRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    log.debug("loading current log level from backend")
    getLogLevel()
      .then((result) => {
        log.debug("log level loaded", { level: result.level })
        setLogLevelState(result.level)
      })
      .catch((e: unknown) => {
        log.error("failed to load log level", e)
        showToast(
          "Failed to load log level: "
            + (e instanceof Error ? e.message : String(e)),
          "error",
        )
      })
  }, [showToast])

  useEffect(() => {
    let es: EventSource | null = null
    let retryTimer: ReturnType<typeof setTimeout> | null = null
    let mounted = true

    const connect = async () => {
      if (!mounted) return
      log.info("setting up log stream")
      setConnecting(true)
      try {
        es = await subscribeToLogs((line) => {
          setLines((prev) => [...prev.slice(-(MAX_LINES - 1)), line])
          setConnected(true)
          setConnecting(false)
        })

        es.addEventListener("open", () => {
          log.info("log stream connected")
          setConnected(true)
          setConnecting(false)
        })

        es.addEventListener("error", () => {
          log.error("log stream error, will retry")
          setConnected(false)
          setConnecting(false)
          if (es) {
            es.close()
            es = null
          }
          if (mounted) {
            retryTimer = setTimeout(connect, 3000)
          }
        })
      } catch (error) {
        log.error("failed to setup log stream, will retry", error)
        setConnected(false)
        setConnecting(false)
        retryTimer = setTimeout(connect, 3000)
      }
    }

    void connect()

    return () => {
      mounted = false
      if (retryTimer) clearTimeout(retryTimer)
      if (es) {
        log.debug("closing log stream")
        es.close()
      }
    }
  }, [showToast])

  useEffect(() => {
    if (!autoScroll) return
    const viewport = logViewportRef.current
    if (!viewport) return
    viewport.scrollTop = viewport.scrollHeight
  }, [autoScroll, lines])

  const scrollToTop = () => {
    const viewport = logViewportRef.current
    if (viewport) {
      viewport.scrollTop = 0
      setAutoScroll(false) // Disable auto-scroll when manually scrolling
    }
  }

  const scrollToBottom = () => {
    const viewport = logViewportRef.current
    if (viewport) {
      viewport.scrollTop = viewport.scrollHeight
      setAutoScroll(true) // Enable auto-scroll when scrolling to bottom
    }
  }

  const clearLogs = () => {
    log.info("clearing visible log buffer")
    setLines([])
    showToast("Cleared visible log buffer")
  }

  const copyLogs = async () => {
    log.debug("copying visible logs to clipboard", { lineCount: lines.length })
    try {
      await navigator.clipboard.writeText(lines.join("\n"))
      showToast("Copied visible logs")
    } catch (e) {
      log.error("clipboard copy failed", e)
      showToast(
        "Copy failed: " + (e instanceof Error ? e.message : String(e)),
        "error",
      )
    }
  }

  const _logLevelLabel =
    LOG_LEVELS.find((level) => level.value === logLevel)?.label ?? "—"

  const card: CSSProperties = {
    background: "var(--color-bg-elevated)",
    borderRadius: "var(--radius-lg)",
    border: "1px solid var(--color-separator)",
    boxShadow: "var(--shadow-card)",
    overflow: "hidden",
  }

  return (
    <div>
      <div
        style={{
          display: "flex",
          alignItems: "flex-start",
          justifyContent: "space-between",
          gap: 18,
          marginBottom: 24,
          flexWrap: "wrap",
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
            Logging
          </h1>
          <p
            style={{
              fontSize: 13,
              color: "var(--color-text-secondary)",
              marginTop: 5,
            }}
          >
            Live proxy output. Real-time streaming of all application logs.
          </p>
        </div>

        <div
          style={{
            display: "flex",
            gap: 8,
            alignItems: "center",
            flexWrap: "wrap",
          }}
        >
          {/* Connection badge */}
          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
            {connecting ?
              <Spin />
            : <span
                className={`status-dot ${connected ? "status-dot-active" : "status-dot-inactive"}`}
              />
            }
            <span
              style={{
                fontSize: 12,
                fontWeight: 600,
                padding: "3px 10px",
                borderRadius: "var(--radius-pill)",
                background:
                  connected ?
                    "var(--color-green-fill)"
                  : "var(--color-orange-fill)",
                color: connected ? "var(--color-green)" : "var(--color-orange)",
              }}
            >
              {/* eslint-disable-next-line no-nested-ternary */}
              {connecting ?
                "Connecting"
              : connected ?
                "Live"
              : "Retrying"}
            </span>
          </div>

          {/* Log Level inline */}
          <select
            value={logLevel ?? "info"}
            onChange={async (e) => {
              const newLevel = e.target.value as LogLevel
              try {
                log.info("changing log level", { from: logLevel, to: newLevel })
                await updateLogLevel(newLevel)
                setLogLevelState(newLevel)
                showToast(
                  `Log level → ${LOG_LEVELS.find((l) => l.value === newLevel)?.label ?? newLevel}`,
                )
              } catch (error) {
                log.error("failed to update log level", error)
                const errMsg =
                  error instanceof Error ? error.message : String(error)
                showToast(`Failed to update log level: ${errMsg}`, "error")
              }
            }}
            className="sys-select"
            style={{
              width: "auto",
              fontSize: 12,
              height: 28,
              padding: "0 28px 0 10px",
            }}
          >
            {LOG_LEVELS.map((level) => (
              <option key={level.value} value={level.value}>
                {level.label}
              </option>
            ))}
          </select>

          <div
            style={{
              width: 1,
              height: 20,
              background: "var(--color-separator-opaque)",
            }}
          />

          <button
            className="btn btn-ghost btn-sm"
            onClick={() => setAutoScroll((prev) => !prev)}
            title={
              autoScroll ?
                "Auto-scroll is on — click to disable"
              : "Auto-scroll is off — click to enable"
            }
          >
            {autoScroll ? "⬇ Auto" : "Manual"}
          </button>
          <button
            className="btn btn-ghost btn-sm"
            onClick={scrollToTop}
            disabled={lines.length === 0}
            title="Scroll to top"
          >
            ↑ Top
          </button>
          <button
            className="btn btn-ghost btn-sm"
            onClick={scrollToBottom}
            disabled={lines.length === 0}
            title="Scroll to bottom"
          >
            ↓ Bottom
          </button>
          <button
            className="btn btn-ghost btn-sm"
            onClick={clearLogs}
            disabled={lines.length === 0}
          >
            Clear
          </button>
          <button
            className="btn btn-ghost btn-sm"
            disabled={lines.length === 0}
            onClick={copyLogs}
          >
            Copy
          </button>
        </div>
      </div>

      {/* Full-width logging area */}
      <section>
        <div style={card}>
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "space-between",
              gap: 12,
              padding: "10px 16px",
              borderBottom: "1px solid var(--color-separator)",
              background: "rgba(255,255,255,0.02)",
            }}
          >
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                color: "var(--color-text-secondary)",
                fontSize: 12,
              }}
            >
              {connecting ?
                <Spin />
              : <span
                  className={`status-dot ${connected ? "status-dot-active" : "status-dot-inactive"}`}
                />
              }
              {/* eslint-disable-next-line no-nested-ternary */}
              {connecting ?
                "Opening stream…"
              : connected ?
                "Receiving new log lines"
              : "Waiting for reconnect"}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 11,
                  color: "var(--color-text-tertiary)",
                }}
              >
                {lines.length}/{MAX_LINES} lines
              </span>
              <span
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 10,
                  color: "var(--color-text-tertiary)",
                  opacity: 0.7,
                }}
              >
                /api/admin/logs/stream
              </span>
            </div>
          </div>

          <div
            ref={logViewportRef}
            style={{
              minHeight: 500,
              maxHeight: "calc(100vh - 260px)",
              overflow: "auto",
              background:
                "linear-gradient(180deg, rgba(255,255,255,0.015), rgba(255,255,255,0.005))",
            }}
          >
            {lines.length === 0 ?
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  minHeight: 500,
                  padding: "36px 24px",
                  textAlign: "center",
                  color: "var(--color-text-secondary)",
                  fontSize: 13,
                }}
              >
                {connecting && (
                  <div
                    style={{ display: "flex", alignItems: "center", gap: 10 }}
                  >
                    <Spin /> Connecting to log stream...
                  </div>
                )}
                {!connecting && "No log lines received yet."}
              </div>
            : <div
                style={{
                  fontFamily: "var(--font-mono)",
                  fontSize: 12,
                  color: "var(--color-text)",
                }}
              >
                {lines.map((line, index) =>
                  (() => {
                    const parsed = parseLogLine(line)
                    const tone = getLogToneStyles(parsed.levelNumber)
                    const segments: Array<ReactNode> = []

                    if (parsed.timestamp) {
                      segments.push(
                        <span
                          key="timestamp"
                          style={{ color: "var(--color-text-tertiary)" }}
                        >
                          {parsed.timestamp}
                        </span>,
                      )
                    }

                    if (parsed.source) {
                      segments.push(
                        <span
                          key="source"
                          style={{ color: tone.accent, fontWeight: 700 }}
                        >
                          {parsed.source}
                        </span>,
                      )
                    }

                    segments.push(
                      <span
                        key="level"
                        style={{ color: tone.accent, fontWeight: 700 }}
                      >
                        {parsed.level}
                      </span>,
                      <span
                        key="message"
                        style={{ color: "var(--color-text)", fontWeight: 600 }}
                      >
                        {parsed.message}
                      </span>,
                    )

                    if (parsed.location) {
                      segments.push(
                        <span
                          key="location"
                          style={{ color: "var(--color-text-secondary)" }}
                        >
                          location={parsed.location}
                        </span>,
                      )
                    }

                    for (const field of parsed.fields) {
                      segments.push(
                        <span
                          key={`${field.key}-${field.value}`}
                          style={{ color: "var(--color-text-secondary)" }}
                        >
                          <span style={{ color: "var(--color-text-tertiary)" }}>
                            {field.key}=
                          </span>
                          <span style={{ color: "var(--color-text)" }}>
                            {field.value}
                          </span>
                        </span>,
                      )
                    }

                    return (
                      <div
                        key={`${index}-${line}`}
                        style={{
                          padding: "8px 14px",
                          borderBottom:
                            index < lines.length - 1 ?
                              "1px solid var(--color-separator)"
                            : "none",
                          borderLeft: `3px solid ${tone.accent}`,
                          background: tone.background,
                          whiteSpace: "pre-wrap",
                          wordBreak: "break-word",
                          overflowX: "auto",
                        }}
                      >
                        <div
                          style={{
                            display: "flex",
                            flexWrap: "wrap",
                            gap: 6,
                            alignItems: "center",
                          }}
                        >
                          {segments.map((segment, segmentIndex) => (
                            <Fragment key={`${index}-${segmentIndex}`}>
                              {segmentIndex > 0 && (
                                <span
                                  style={{
                                    color: "var(--color-text-tertiary)",
                                  }}
                                >
                                  |
                                </span>
                              )}
                              {segment}
                            </Fragment>
                          ))}
                        </div>
                      </div>
                    )
                  })(),
                )}
              </div>
            }
          </div>
        </div>
      </section>
    </div>
  )
}
