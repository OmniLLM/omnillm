#!/usr/bin/env bun
// omni-dev.ts — Comprehensive OmniModel Development Manager
// Manages both frontend and backend services with start/stop/status operations

import { parseArgs } from "node:util"
import consola from "consola"
import {
  appendFileSync,
  existsSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs"
import { join } from "node:path"
import { homedir } from "node:os"

const PID_FILE = join(process.cwd(), ".omni-dev.pid")
const LOG_FILE = join(process.cwd(), ".omni-dev.log")

interface ServicePids {
  backend?: number
  frontend?: number
}

type ProcessInfo = {
  pid: number
  cmd: string
}

type StructuredLogPayload = Record<string, unknown>

const ANSI_PATTERN = /\u001b\[[0-9;]*m/g
const STRUCTURED_FIELD_ORDER = [
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
const SOURCE_COLORS: Record<string, string> = {
  backend: "31",
  frontend: "32",
}
const LEVEL_COLORS: Record<string, string> = {
  FATAL: "31",
  ERROR: "31",
  WARN: "33",
  INFO: "36",
  DEBUG: "35",
  TRACE: "90",
}

const { values, positionals } = parseArgs({
  args: Bun.argv.slice(2),
  allowPositionals: true,
  options: {
    "server-port": { type: "string", default: "5002" },
    "frontend-port": { type: "string", default: "5080" },
    "backend": { type: "string", default: "go" }, // go or node
    "help": { type: "boolean", short: "h" },
    "verbose": { type: "boolean", short: "v" },
    "rebuild": { type: "boolean", short: "r" },
  },
})

const command = positionals[0] || "help"
const serverPort = values["server-port"]
const frontendPort = values["frontend-port"]
const backend = values["backend"]
const verbose = values["verbose"]
const rebuild = values["rebuild"]

function showHelp() {
  console.log(`
🚀 OmniModel Development Manager

USAGE:
  bun run omni-dev.ts <command> [options]

COMMANDS:
  start          Start both frontend and backend services
  stop           Stop all running services
  restart        Stop and start services
  restart --rebuild    Stop, rebuild frontend and backend, then start
  status         Show service status and ports
  logs           Show recent service logs
  help           Show this help message

OPTIONS:
  --server-port <port>    Backend server port (default: 5002)
  --frontend-port <port>  Frontend dev server port (default: 5080)
  --backend <type>        Backend type: 'go' or 'node' (default: go)
  --verbose, -v           Enable verbose logging
  --rebuild, -r           Stop services, rebuild both frontend and backend, then start
  --help, -h              Show help

EXAMPLES:
  # Start with default ports (backend: 5002, frontend: 5080)
  bun run omni-dev.ts start

  # Start with custom ports
  bun run omni-dev.ts start --server-port 5000 --frontend-port 5080

  # Rebuild and restart with custom ports
  bun run omni-dev.ts restart --rebuild --server-port 5000 --frontend-port 5080

  # Start with TypeScript backend instead of Go
  bun run omni-dev.ts start --backend node

  # Check service status
  bun run omni-dev.ts status

  # Stop all services
  bun run omni-dev.ts stop

  # View logs
  bun run omni-dev.ts logs

SERVICE ENDPOINTS:
  Backend API:     http://localhost:${serverPort}
  Frontend:        http://localhost:${frontendPort}
  Admin UI:        http://localhost:${frontendPort}/admin/

FEATURES:
  • 🔥 High-performance Golang backend (default)
  • 🟦 TypeScript/Node.js backend (alternative)
  • 🌐 React frontend with Vite dev server
  • 📱 Integrated admin UI
  • 🔗 Automatic API proxying
  • 🔄 Hot reload for development
  • 📊 Real-time service monitoring
  • 🛑 Graceful service shutdown

NOTES:
  • Services run in the background and persist across terminal sessions
  • PID tracking ensures clean startup/shutdown
  • Go binary is built to ~/.local/bin for clean project structure
  • Frontend automatically proxies API calls to backend
  • All services stop gracefully with proper cleanup
`)
}

function savePids(pids: ServicePids) {
  writeFileSync(PID_FILE, JSON.stringify(pids, null, 2))
  if (verbose) consola.info(`PIDs saved to ${PID_FILE}`)
}

function loadPids(): ServicePids | null {
  if (!existsSync(PID_FILE)) return null
  try {
    const content = readFileSync(PID_FILE, "utf8")
    return JSON.parse(content) as ServicePids
  } catch {
    return null
  }
}

function clearPids() {
  if (existsSync(PID_FILE)) {
    unlinkSync(PID_FILE)
    if (verbose) consola.info("PID file cleaned up")
  }
}

function isProcessRunning(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

async function listProcesses(): Promise<ProcessInfo[]> {
  // Cross-platform process listing with minimal dependencies
  if (process.platform === "win32") {
    const proc = Bun.spawn([
      "powershell",
      "-NoProfile",
      "-Command",
      "Get-CimInstance Win32_Process | Select-Object ProcessId,CommandLine | ConvertTo-Json"
    ], { stdout: "pipe", stderr: "pipe" })

    const output = await new Response(proc.stdout).text()
    try {
      const rows = JSON.parse(output) as Array<{ ProcessId: number; CommandLine?: string }>
      return rows
        .filter(row => typeof row.ProcessId === "number")
        .map(row => ({ pid: row.ProcessId, cmd: row.CommandLine || "" }))
    } catch {
      return []
    }
  }

  const proc = Bun.spawn([
    "ps",
    "-eo",
    "pid=,command=",
  ], { stdout: "pipe", stderr: "pipe" })

  const output = await new Response(proc.stdout).text()
  return output
    .split("\n")
    .map(line => line.trim())
    .filter(Boolean)
    .map(line => {
      const [pidStr, ...cmdParts] = line.split(/\s+/)
      return {
        pid: Number(pidStr),
        cmd: cmdParts.join(" "),
      }
    })
    .filter(p => !Number.isNaN(p.pid))
}

function matchesProcess(cmd: string, keywords: string[]): boolean {
  const lower = cmd.toLowerCase()
  return keywords.every(keyword => lower.includes(keyword.toLowerCase()))
}

async function findMatchingPids(): Promise<number[]> {
  const processes = await listProcesses()

  // Keywords chosen to uniquely identify OmniModel dev processes
  // Use path-separator-agnostic keywords (avoid / vs \ issues on Windows)
  const patterns: Array<string[]> = [
    ["omnimodel", "start", "--port"], // Go binary
    ["src/main.ts", "start", "--port"], // TS backend via bun
    ["vite.config.ts", "--port"], // Frontend Vite (matches forward or backslash paths)
  ]

  const matching = new Set<number>()

  for (const processInfo of processes) {
    if (processInfo.pid === process.pid) continue // Skip self

    for (const keywords of patterns) {
      if (matchesProcess(processInfo.cmd, keywords)) {
        matching.add(processInfo.pid)
        break
      }
    }
  }

  return Array.from(matching)
}

async function checkPortAvailable(port: number): Promise<boolean> {
  try {
    await Bun.connect({
      hostname: "127.0.0.1",
      port,
      socket: {
        open: (s) => s.end(),
        data: () => {},
        close: () => {},
        error: () => {},
      },
    })
    return false // Port is occupied
  } catch {
    return true // Port is available
  }
}

function stripAnsi(value: string): string {
  return value.replaceAll(ANSI_PATTERN, "")
}

function normalizeSourceLabel(label: string): string {
  if (label === "go-backend" || label === "ts-backend") {
    return "backend"
  }

  return label
}

function stringifyLogValue(value: unknown): string {
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

function formatStructuredField(key: string, value: unknown): string | null {
  const formattedValue = stringifyLogValue(value)
  if (!formattedValue) {
    return null
  }

  switch (key) {
    case "request_id": {
      return `request=${formattedValue}`
    }
    case "api_shape": {
      return `api=${formattedValue}`
    }
    case "model_requested": {
      return `requested=${formattedValue}`
    }
    case "model_used": {
      return `used=${formattedValue}`
    }
    case "latency_ms": {
      return `latency=${formattedValue}ms`
    }
    case "input_tokens": {
      return `input=${formattedValue}`
    }
    case "output_tokens": {
      return `output=${formattedValue}`
    }
    case "level":
    case "message":
    case "time": {
      return null
    }
    default: {
      return `${key}=${formattedValue}`
    }
  }
}

function formatStructuredLogLine(
  source: string,
  payload: StructuredLogPayload,
  fallbackTimestamp: string,
  fallbackLevel: string,
): string {
  const timestamp =
    typeof payload.time === "string" && payload.time.trim() ?
      payload.time.trim()
    : fallbackTimestamp
  const level =
    typeof payload.level === "string" && payload.level.trim() ?
      payload.level.trim().toUpperCase()
    : fallbackLevel
  const message =
    typeof payload.message === "string" && payload.message.trim() ?
      payload.message.trim()
    : JSON.stringify(payload)

  const segments = [`[${timestamp}]`, source, level, message]
  const seen = new Set<string>()

  for (const key of STRUCTURED_FIELD_ORDER) {
    const formatted = formatStructuredField(key, payload[key])
    if (formatted) {
      segments.push(formatted)
      seen.add(key)
    }
  }

  const remaining: string[] = []
  for (const [key, value] of Object.entries(payload)) {
    if (seen.has(key)) {
      continue
    }

    const formatted = formatStructuredField(key, value)
    if (formatted) {
      remaining.push(formatted)
    }
  }

  remaining.sort((left, right) => left.localeCompare(right))
  return segments.concat(remaining).join(" | ")
}

function inferTextLogLevel(line: string, isError: boolean): string {
  const lower = line.toLowerCase()

  if (isError || lower.includes("error") || lower.includes("failed")) {
    return "ERROR"
  }

  if (lower.includes("warn")) {
    return "WARN"
  }

  if (lower.includes("trace")) {
    return "TRACE"
  }

  if (lower.includes("debug")) {
    return "DEBUG"
  }

  return "INFO"
}

function colorizeSegment(value: string, color: string | undefined): string {
  if (!color) {
    return value
  }

  return `\x1b[${color}m${value}\x1b[0m`
}

function colorizeLogLine(line: string, source: string, level: string): string {
  const segments = line.split(" | ")
  if (segments.length < 4) {
    return line
  }

  const [timestamp, sourceSegment, levelSegment, ...rest] = segments
  return [
    timestamp,
    colorizeSegment(sourceSegment ?? source, SOURCE_COLORS[source]),
    colorizeSegment(levelSegment ?? level, LEVEL_COLORS[level]),
    ...rest,
  ].join(" | ")
}

function formatProcessLogLine(
  label: string,
  rawLine: string,
  receivedAt: string,
  isError: boolean,
): { fileEntry: string; consoleEntry: string } {
  const source = normalizeSourceLabel(label)
  const cleanLine = stripAnsi(rawLine).trim()

  let fileEntry: string
  try {
    const parsed = JSON.parse(cleanLine) as unknown
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      fileEntry = formatStructuredLogLine(
        source,
        parsed as StructuredLogPayload,
        receivedAt,
        inferTextLogLevel(cleanLine, isError),
      )

      const levelSegment = fileEntry.split(" | ")[2] ?? "INFO"
      return {
        fileEntry,
        consoleEntry: colorizeLogLine(fileEntry, source, levelSegment),
      }
    }
  } catch {
    // Plain text process output is formatted below.
  }

  const level = inferTextLogLevel(cleanLine, isError)
  fileEntry = [`[${receivedAt}]`, source, level, cleanLine].join(" | ")
  return {
    fileEntry,
    consoleEntry: colorizeLogLine(fileEntry, source, level),
  }
}

function createLoggedProcess(
  label: string,
  options: { color: string; cmd: string; args: Array<string> },
) {
  const env = {
    ...process.env,
    GO_PORT: serverPort,
    SERVER_PORT: serverPort,
    FRONTEND_PORT: frontendPort,
  }

  const proc = Bun.spawn([options.cmd, ...options.args], {
    env,
    stdout: "pipe",
    stderr: "pipe",
    detached: true,
  })

  function logOutput(stream: ReadableStream<Uint8Array>, isError = false) {
    void (async () => {
      const reader = stream.getReader()
      const decoder = new TextDecoder()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        const text = decoder.decode(value)

        // Filter out verbose logs
        const lines = text.split("\n").filter(line => line.trim())
        for (const line of lines) {
          // Filter out noisy Gin debug routes and warnings unless verbose
          if (!verbose) {
            if (line.includes("[GIN-debug]") ||
                line.includes("trusted all proxies") ||
                line.includes("pkg.go.dev/github.com/gin-gonic/gin") ||
                line.includes("Creating an Engine instance")) {
              continue
            }
          }

          const timestamp = new Date().toISOString()
          const { fileEntry, consoleEntry } = formatProcessLogLine(
            label,
            line,
            timestamp,
            isError,
          )

          // Write to log file
          try {
            appendFileSync(LOG_FILE, `${fileEntry}\n`)
          } catch {
            // ignore log file write errors
          }

          // Print to console if verbose or if it's startup info
          if (verbose || line.includes("running at") || line.includes("ready in")) {
            process[isError ? "stderr" : "stdout"].write(`${consoleEntry}\n`)
          }
        }
      }
    })()
  }

  logOutput(proc.stdout, false)
  logOutput(proc.stderr, true)

  return proc
}

async function startServices() {
  const existingPids = loadPids()
  if (existingPids) {
    const backendRunning = existingPids.backend && isProcessRunning(existingPids.backend)
    const frontendRunning = existingPids.frontend && isProcessRunning(existingPids.frontend)

    if (backendRunning || frontendRunning) {
      consola.warn("Services are already running. Use 'stop' first or 'restart' to restart them.")
      return await showStatus()
    }
  }

  consola.info("🚀 Starting OmniModel development environment...")
  consola.info(`   🔥 Backend (${backend.toUpperCase()}): http://localhost:${serverPort}`)
  consola.info(`   🌐 Frontend: http://localhost:${frontendPort}`)
  consola.info(`   📱 Admin UI: http://localhost:${frontendPort}/admin/`)
  if (!verbose) {
    consola.info(`   📝 Logs: ${LOG_FILE}`)
    consola.info(`   💡 Use --verbose to see real-time logs`)
  }
  console.log()

  // Check ports
  const serverAvailable = await checkPortAvailable(Number(serverPort))

  if (!serverAvailable) {
    consola.error(`❌ Port ${serverPort} is already in use by another service`)
    consola.info(`💡 Try a different port: ./omni start --server-port 5003`)
    process.exit(1)
  }

  // Note: We don't strictly check frontend port as Vite will auto-find an available one

  // Clear old logs
  if (existsSync(LOG_FILE)) {
    writeFileSync(LOG_FILE, "")
  }

  const bunExe = process.execPath
  let backendProc: ReturnType<typeof Bun.spawn>

  if (backend === "go") {
    // Ensure Go binary exists
    const isWindows = process.platform === "win32"
    const binaryPath = isWindows
      ? `${process.env.USERPROFILE}/.local/bin/omnimodel.exe`
      : `${homedir()}/.local/bin/omnimodel`

    consola.info("🔨 Building Golang backend...")
    const goExe = process.platform === "win32"
      ? `C:\\Program Files\\Go\\bin\\go.exe`
      : `go`
    // Using modernc.org/sqlite (pure Go) instead of go-sqlite3, so no CGO needed
    const buildResult = Bun.spawn([goExe, "build", "-o", binaryPath, "main.go"], {
      stdout: "inherit",
      stderr: "inherit",
    })
    await buildResult.exited
    if (buildResult.exitCode !== 0) {
      consola.error("❌ Failed to build Golang backend")
      process.exit(1)
    }
    consola.success("✅ Golang backend built successfully")

    backendProc = createLoggedProcess("go-backend", {
      color: "31",
      cmd: binaryPath,
      args: ["start", "--port", serverPort],
    })
  } else {
    backendProc = createLoggedProcess("ts-backend", {
      color: "34",
      cmd: bunExe,
      args: ["--watch", "src/main.ts", "start", "--port", serverPort],
    })
  }

  // Wait a bit for backend to start
  await Bun.sleep(2000)

  const frontendProc = createLoggedProcess("frontend", {
    color: "32",
    cmd: bunExe,
    args: ["node_modules/.bin/vite", "--config", "frontend/vite.config.ts", "--port", frontendPort],
  })

  // Save PIDs
  savePids({
    backend: backendProc.pid,
    frontend: frontendProc.pid,
  })

  consola.success("🎉 Services started successfully!")
  consola.info("💡 Use 'bun run omni-dev.ts stop' to stop services")
}

async function killPid(pid: number): Promise<boolean> {
  if (process.platform === "win32") {
    // /F = force, /T = kill entire process tree (catches Vite's child processes)
    const result = Bun.spawn(["taskkill", "/F", "/T", "/PID", String(pid)], {
      stdout: "pipe",
      stderr: "pipe",
    })
    await result.exited
    return result.exitCode === 0
  }
  try {
    // Try SIGTERM first, fall back to SIGKILL
    process.kill(pid, "SIGTERM")
    return true
  } catch {
    try {
      process.kill(pid, "SIGKILL")
      return true
    } catch {
      return false
    }
  }
}

async function stopServices() {
  const pids = loadPids()
  consola.info("🛑 Stopping services...")

  const killed = new Set<number>()

  // First stop tracked processes
  if (pids?.backend && isProcessRunning(pids.backend)) {
    const ok = await killPid(pids.backend)
    if (ok) {
      killed.add(pids.backend)
      consola.success(`✅ Backend stopped (PID: ${pids.backend})`)
    } else {
      consola.warn(`⚠️  Could not stop backend (PID: ${pids.backend})`)
    }
  }

  if (pids?.frontend && isProcessRunning(pids.frontend)) {
    const ok = await killPid(pids.frontend)
    if (ok) {
      killed.add(pids.frontend)
      consola.success(`✅ Frontend stopped (PID: ${pids.frontend})`)
    } else {
      consola.warn(`⚠️  Could not stop frontend (PID: ${pids.frontend})`)
    }
  }

  // Then find any stray OmniModel processes by command signature
  const matchingPids = await findMatchingPids()
  const extraPids = matchingPids.filter(pid => !killed.has(pid))

  for (const pid of extraPids) {
    const ok = await killPid(pid)
    if (ok) {
      killed.add(pid)
      consola.success(`✅ Stopped stray process (PID: ${pid})`)
    } else {
      consola.warn(`⚠️  Could not stop stray process (PID: ${pid})`)
    }
  }

  clearPids()

  if (killed.size === 0) {
    consola.info("ℹ️  No running services found")
  } else {
    consola.success(`🎉 Stopped ${killed.size} process${killed.size > 1 ? 'es' : ''}`)
  }
}

async function showStatus() {
  const pids = loadPids()

  consola.info("📊 OmniModel Service Status")
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

  if (!pids) {
    consola.info("📭 No services are currently tracked")
    console.log()
    consola.info("💡 Use 'bun run omni-dev.ts start' to start services")
    return
  }

  // Backend status
  const backendRunning = pids.backend && isProcessRunning(pids.backend)
  const backendPortBusy = !(await checkPortAvailable(Number(serverPort)))

  console.log(`🔥 Backend (${backend.toUpperCase()}):`)
  console.log(`   Status: ${backendRunning ? '🟢 Running' : '🔴 Stopped'}`)
  console.log(`   PID: ${pids.backend || 'N/A'}`)
  console.log(`   Port: ${serverPort} ${backendPortBusy ? '(🔒 Busy)' : '(🔓 Free)'}`)
  console.log(`   URL: http://localhost:${serverPort}`)

  // Frontend status
  const frontendRunning = pids.frontend && isProcessRunning(pids.frontend)
  const frontendPortBusy = !(await checkPortAvailable(Number(frontendPort)))

  console.log(`🌐 Frontend:`)
  console.log(`   Status: ${frontendRunning ? '🟢 Running' : '🔴 Stopped'}`)
  console.log(`   PID: ${pids.frontend || 'N/A'}`)
  console.log(`   Port: ${frontendPort} ${frontendPortBusy ? '(🔒 Busy)' : '(🔓 Free)'}`)
  console.log(`   URL: http://localhost:${frontendPort}`)
  console.log(`   Admin UI: http://localhost:${frontendPort}/admin/`)

  console.log()

  if (backendRunning && frontendRunning) {
    consola.success("🎉 All services are running!")
  } else if (!backendRunning && !frontendRunning) {
    consola.info("💤 All services are stopped")
    consola.info("💡 Use 'bun run omni-dev.ts start' to start them")
  } else {
    consola.warn("⚠️  Some services are not running")
    consola.info("💡 Use 'bun run omni-dev.ts restart' to restart all")
  }
}

function showLogs() {
  if (!existsSync(LOG_FILE)) {
    consola.info("📭 No logs available yet")
    consola.info("💡 Start services first to generate logs")
    return
  }

  try {
    const logs = readFileSync(LOG_FILE, "utf8")
    const lines = logs.split("\n").filter(line => line.trim())
    const recentLines = lines.slice(-50) // Show last 50 lines

    consola.info("📋 Recent Service Logs (last 50 lines):")
    console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    console.log(recentLines.join("\n"))
    console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
    consola.info(`💡 Full logs: ${LOG_FILE}`)
  } catch (error) {
    consola.error("❌ Could not read log file")
  }
}

// Main command handling
if (values.help || command === "help") {
  showHelp()
} else if (command === "start") {
  await startServices()
} else if (command === "stop") {
  await stopServices()
} else if (command === "restart") {
  await stopServices()
  await Bun.sleep(2000)
  if (rebuild) {
    // Rebuild Go backend
    if (backend === "go") {
      const isWindows = process.platform === "win32"
      const binaryPath = isWindows
        ? `${process.env.USERPROFILE}/.local/bin/omnimodel.exe`
        : `${homedir()}/.local/bin/omnimodel`
      consola.info("🔨 Rebuilding Golang backend...")
      const goExe = process.platform === "win32"
        ? `C:\\Program Files\\Go\\bin\\go.exe`
        : `go`
      const buildResult = Bun.spawn([goExe, "build", "-o", binaryPath, "main.go"], {
        stdout: "inherit",
        stderr: "inherit",
      })
      await buildResult.exited
      if (buildResult.exitCode !== 0) {
        consola.error("❌ Failed to rebuild Golang backend")
        process.exit(1)
      }
      consola.success("✅ Golang backend rebuilt successfully")
    }
    // Rebuild frontend
    consola.info("🔨 Rebuilding frontend...")
    const buildResult = Bun.spawn([process.execPath, "run", "build:frontend"], {
      stdout: "inherit",
      stderr: "inherit",
    })
    await buildResult.exited
    if (buildResult.exitCode !== 0) {
      consola.error("❌ Failed to rebuild frontend")
      process.exit(1)
    }
    consola.success("✅ Frontend rebuilt successfully")
  }
  await startServices()
} else if (command === "status") {
  await showStatus()
} else if (command === "logs") {
  showLogs()
} else {
  consola.error(`❌ Unknown command: ${command}`)
  console.log()
  showHelp()
  process.exit(1)
}

// Graceful shutdown handling
process.on("SIGINT", async () => {
  consola.info("\n🛑 Received interrupt signal...")
  await stopServices()
  process.exit(0)
})

process.on("SIGTERM", async () => {
  consola.info("\n🛑 Received termination signal...")
  await stopServices()
  process.exit(0)
})
