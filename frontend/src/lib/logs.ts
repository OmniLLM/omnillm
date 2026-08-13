// eslint-disable-next-line no-control-regex -- intentional: strip ANSI escape codes
const ANSI_PATTERN = /\u001b\[[0-9;]*m/g
const PIPE_SEPARATOR = " | "
const LEGACY_LOG_PATTERN =
  /^\[([^\]]+)\] \[LOG-(\d+)\] ([A-Z]+)(\s+\[[^\]]+\])?: (.+)$/
const WRAPPED_JSON_PATTERN = /^\[([^\]]+)\]\s+\[([^\]]+)\]\s+(\{.+\})$/

const FIELD_ORDER = [
  "request_id",
  "api_shape",
  "model_requested",
  "model_used",
  "model",
  "provider",
  "messages",
  "tools",
  "stream",
  "stop_reason",
  "input_tokens",
  "output_tokens",
  "method",
  "path",
  "status",
  "latency_ms",
  "url",
  "admin",
  "count",
  "verbose",
] as const

const FIELD_ALIASES: Record<string, string> = {
  request_id: "request",
  api_shape: "api",
  model_requested: "requested",
  model_used: "used",
  input_tokens: "input",
  output_tokens: "output",
  latency_ms: "latency",
}

const LOG_LEVEL_NUMBERS: Record<string, number> = {
  FATAL: 0,
  ERROR: 1,
  WARN: 2,
  INFO: 3,
  DEBUG: 4,
  TRACE: 5,
}

export interface ParsedLogField {
  key: string
  value: string
}

export interface ParsedLogLine {
  raw: string
  timestamp: string | null
  source: string | null
  level: string
  levelNumber: number
  message: string
  location: string | null
  fields: Array<ParsedLogField>
}

function stripAnsi(value: string): string {
  return value.replaceAll(ANSI_PATTERN, "")
}

function normalizeSource(source: string | null | undefined): string | null {
  if (!source) {
    return null
  }

  if (source === "go-backend" || source === "ts-backend") {
    return "backend"
  }

  return source
}

function stringifyValue(value: unknown): string {
  if (typeof value === "string") {
    return value.trim()
  }

  if (typeof value === "number" || typeof value === "boolean") {
    return String(value)
  }

  if (value === null || value === undefined) {
    return ""
  }

  try {
    return JSON.stringify(value)
  } catch {
    return Object.prototype.toString.call(value)
  }
}

function normalizeLevel(level: string | null | undefined): string {
  const normalized = level?.trim().toUpperCase()
  if (!normalized) {
    return "INFO"
  }

  if (normalized === "WARNING") {
    return "WARN"
  }

  return normalized
}

function getLevelNumber(level: string): number {
  return LOG_LEVEL_NUMBERS[normalizeLevel(level)] ?? 3
}

function formatField(key: string, value: unknown): ParsedLogField | null {
  if (
    key === "level"
    || key === "message"
    || key === "time"
    || key === "timestamp"
    || key === "type"
  ) {
    return null
  }

  const formattedValue = stringifyValue(value)
  if (!formattedValue) {
    return null
  }

  if (key === "latency_ms" && !formattedValue.endsWith("ms")) {
    return {
      key: FIELD_ALIASES[key] ?? key,
      value: `${formattedValue}ms`,
    }
  }

  return {
    key: FIELD_ALIASES[key] ?? key,
    value: formattedValue,
  }
}

function collectStructuredFields(
  payload: Record<string, unknown>,
): Array<ParsedLogField> {
  const fields: Array<ParsedLogField> = []
  const seen = new Set<string>()

  for (const key of FIELD_ORDER) {
    const formatted = formatField(key, payload[key])
    if (formatted) {
      fields.push(formatted)
      seen.add(key)
    }
  }

  const remaining: Array<ParsedLogField> = []
  for (const [key, value] of Object.entries(payload)) {
    if (seen.has(key)) {
      continue
    }

    const formatted = formatField(key, value)
    if (formatted) {
      remaining.push(formatted)
    }
  }

  remaining.sort((left, right) =>
    `${left.key}=${left.value}`.localeCompare(`${right.key}=${right.value}`),
  )

  return fields.concat(remaining)
}

function parseStructuredPayload(
  payload: Record<string, unknown>,
  fallbackTimestamp: string | null,
  fallbackSource: string | null,
): ParsedLogLine {
  let timestamp = fallbackTimestamp
  if (typeof payload.time === "string" && payload.time.trim()) {
    timestamp = payload.time.trim()
  } else if (
    typeof payload.timestamp === "string"
    && payload.timestamp.trim()
  ) {
    timestamp = payload.timestamp.trim()
  }

  const source = normalizeSource(fallbackSource ?? "backend")
  const level = normalizeLevel(
    typeof payload.level === "string" ? payload.level : null,
  )
  const message =
    typeof payload.message === "string" && payload.message.trim() ?
      payload.message.trim()
    : JSON.stringify(payload)

  return {
    raw: JSON.stringify(payload),
    timestamp,
    source,
    level,
    levelNumber: getLevelNumber(level),
    message,
    location: null,
    fields: collectStructuredFields(payload),
  }
}

function parsePipeLogLine(line: string): ParsedLogLine | null {
  const segments = stripAnsi(line)
    .trim()
    .split(PIPE_SEPARATOR)
    .map((segment) => segment.trim())

  if (segments.length < 4) {
    return null
  }

  const [
    timestampSegment,
    sourceSegment,
    levelSegment,
    messageSegment,
    ...fieldSegments
  ] = segments

  if (!timestampSegment.startsWith("[") || !timestampSegment.endsWith("]")) {
    return null
  }

  return {
    raw: line,
    timestamp: timestampSegment.slice(1, -1),
    source: normalizeSource(sourceSegment),
    level: normalizeLevel(levelSegment),
    levelNumber: getLevelNumber(levelSegment),
    message: messageSegment,
    location: null,
    fields: fieldSegments
      .map((fieldSegment) => {
        const separatorIndex = fieldSegment.indexOf("=")
        if (separatorIndex === -1) {
          return {
            key: "detail",
            value: fieldSegment,
          }
        }

        return {
          key: fieldSegment.slice(0, separatorIndex),
          value: fieldSegment.slice(separatorIndex + 1),
        }
      })
      .filter((field) => field.value.trim() !== ""),
  }
}

function parseLegacyLogLine(line: string): ParsedLogLine | null {
  const match = stripAnsi(line).trim().match(LEGACY_LOG_PATTERN)
  if (!match) {
    return null
  }

  const [, timestamp, levelNumber, level, location, message] = match
  return {
    raw: line,
    timestamp,
    source: null,
    level: normalizeLevel(level),
    levelNumber: Number.parseInt(levelNumber, 10),
    message,
    location: location ? location.trim() : null,
    fields: [],
  }
}

function parseWrappedJsonLogLine(line: string): ParsedLogLine | null {
  const match = stripAnsi(line).trim().match(WRAPPED_JSON_PATTERN)
  if (!match) {
    return null
  }

  const [, timestamp, source, payloadText] = match

  try {
    const payload = JSON.parse(payloadText) as unknown
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
      return null
    }

    const parsed = parseStructuredPayload(
      payload as Record<string, unknown>,
      timestamp,
      source,
    )
    return {
      ...parsed,
      raw: line,
      timestamp,
    }
  } catch {
    return null
  }
}

function parseRawJsonLogLine(line: string): ParsedLogLine | null {
  const normalized = stripAnsi(line).trim()
  if (!normalized.startsWith("{") || !normalized.endsWith("}")) {
    return null
  }

  try {
    const payload = JSON.parse(normalized) as unknown
    if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
      return null
    }

    return {
      ...parseStructuredPayload(
        payload as Record<string, unknown>,
        null,
        "backend",
      ),
      raw: line,
    }
  } catch {
    return null
  }
}

function inferPlainLogLevel(line: string): string {
  const normalized = line.toLowerCase()
  if (normalized.includes("error") || normalized.includes("failed")) {
    return "ERROR"
  }

  if (normalized.includes("warn")) {
    return "WARN"
  }

  if (normalized.includes("trace")) {
    return "TRACE"
  }

  if (normalized.includes("debug")) {
    return "DEBUG"
  }

  return "INFO"
}

export function parseLogLine(line: string): ParsedLogLine {
  const plainLine = stripAnsi(line).trim()
  const plainLevel = inferPlainLogLevel(plainLine)

  return (
    parsePipeLogLine(line)
    ?? parseLegacyLogLine(line)
    ?? parseWrappedJsonLogLine(line)
    ?? parseRawJsonLogLine(line) ?? {
      raw: line,
      timestamp: null,
      source: null,
      level: plainLevel,
      levelNumber: getLevelNumber(plainLevel),
      message: plainLine,
      location: null,
      fields: [],
    }
  )
}
