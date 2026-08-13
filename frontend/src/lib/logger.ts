// Structured frontend logger with log levels.
// All logs go through this utility so they can be filtered, formatted, or
// shipped to a remote sink in the future.

export type LogLevel = "fatal" | "error" | "warn" | "info" | "debug" | "trace"

const LEVEL_NUMBERS: Record<LogLevel, number> = {
  fatal: 0,
  error: 1,
  warn: 2,
  info: 3,
  debug: 4,
  trace: 5,
}

// Global minimum level — anything below this number is dropped.
// Defaults to "info" in production, "debug" in development.
const currentLevel: LogLevel = import.meta.env.DEV ? "debug" : "info"

function shouldLog(level: LogLevel): boolean {
  return LEVEL_NUMBERS[level] <= LEVEL_NUMBERS[currentLevel]
}

function formatMessage(level: string, message: string, meta: unknown): string {
  const timestamp = new Date().toISOString()
  const metaStr = meta ? ` ${JSON.stringify(meta)}` : ""
  return `[${timestamp}] [FE-${level.toUpperCase()}]${metaStr} ${message}`
}

function emit(level: LogLevel, message: string, meta?: unknown): void {
  if (!shouldLog(level)) return
  const formatted = formatMessage(level, message, meta)
  // eslint-disable-next-line default-case -- LogLevel union is exhaustive
  switch (level) {
    case "fatal":
    case "error": {
      console.error(formatted)
      break
    }
    case "warn": {
      console.warn(formatted)
      break
    }
    case "info": {
      console.info(formatted)
      break
    }
    case "debug": {
      console.debug(formatted)
      break
    }
    case "trace": {
      console.trace(formatted)
      break
    }
  }
}

/**
 * Create a namespaced logger.
 * Example:
 *   const log = createLogger("api")
 *   log.info("fetching providers")
 *   log.error("request failed", { url: "/api/providers" })
 */
export function createLogger(namespace: string) {
  return {
    fatal: (message: string, meta?: unknown) =>
      emit("fatal", `[${namespace}] ${message}`, meta),
    error: (message: string, meta?: unknown) =>
      emit("error", `[${namespace}] ${message}`, meta),
    warn: (message: string, meta?: unknown) =>
      emit("warn", `[${namespace}] ${message}`, meta),
    info: (message: string, meta?: unknown) =>
      emit("info", `[${namespace}] ${message}`, meta),
    debug: (message: string, meta?: unknown) =>
      emit("debug", `[${namespace}] ${message}`, meta),
    trace: (message: string, meta?: unknown) =>
      emit("trace", `[${namespace}] ${message}`, meta),
  }
}
