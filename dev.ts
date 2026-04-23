#!/usr/bin/env bun
// dev.ts — dev launcher for Go backend + frontend
// Usage: bun run dev [--server-port 5002] [--frontend-port 5080] [--host 127.0.0.1]

import consola from "consola"
import { homedir } from "node:os"
import { parseArgs } from "node:util"

const { values } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    "server-port": { type: "string", default: "5002" },
    "frontend-port": { type: "string", default: "5080" },
    host: { type: "string", default: "127.0.0.1" },
  },
})

const serverPort = values["server-port"]
const frontendPort = values["frontend-port"]
const host = values.host

const env = {
  ...process.env,
  GO_PORT: serverPort,
  SERVER_PORT: serverPort,
  FRONTEND_PORT: frontendPort,
  HOST: host,
}

consola.info(`🚀 Starting development environment:`)
consola.info(`   🔥 Golang backend: http://${host}:${serverPort}`)
consola.info(`   🌐 Frontend dev server: http://${host}:${frontendPort}`)
consola.info(`   📱 Admin UI: http://${host}:${frontendPort}/admin/`)
consola.info(``)

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

function run(label: string, color: string, cmd: string, args: Array<string>) {
  const labelColor = `\x1b[${color}m`
  const resetColor = `\x1b[0m`
  const proc = Bun.spawn([cmd, ...args], {
    env,
    stdout: "pipe",
    stderr: "pipe",
  })

  function pipe(
    stream: ReadableStream<Uint8Array>,
    out: typeof process.stdout,
  ) {
    // eslint-disable-next-line @typescript-eslint/no-floating-promises
    ;(async () => {
      const reader = stream.getReader()
      const decoder = new TextDecoder()
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        const text = decoder.decode(value)

        // Filter out verbose MCP tool schema logging
        const lines = text.split("\n")
        const filteredLines = lines.filter((line) => {
          const isVerboseToolSchema =
            line.includes('"type": "function"')
            && line.includes('"description":')
            && line.includes('"parameters":')
          return !isVerboseToolSchema
        })

        if (filteredLines.length > 0 && filteredLines.join("\n").trim()) {
          const output = filteredLines.join("\n")
          const processedOutput = output
            .split("\n")
            .map((line) => {
              if (line.trim()) {
                return `${labelColor}[${label}]${resetColor} ${line}`
              }
              return line
            })
            .join("\n")

          out.write(processedOutput)
        }
      }
    })()
  }

  pipe(proc.stdout, process.stdout)
  pipe(proc.stderr, process.stderr)
  return proc
}

async function waitForPort(port: number, timeoutMs = 30_000): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
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
      return
    } catch {
      await Bun.sleep(100)
    }
  }
  throw new Error(`Server did not start on port ${port} within ${timeoutMs}ms`)
}

async function preflightCheck() {
  const serverAvailable = await checkPortAvailable(Number(serverPort))
  const frontendAvailable = await checkPortAvailable(Number(frontendPort))

  if (!serverAvailable) {
    consola.warn(
      `⚠️  Port ${serverPort} is already in use - backend may fail to start`,
    )
  }
  if (!frontendAvailable) {
    consola.warn(
      `⚠️  Port ${frontendPort} is already in use - frontend may fail to start`,
    )
  }
}

await preflightCheck()

const bunExe = process.execPath

const isWindows = process.platform === "win32"
const binaryPath =
  isWindows ?
    `${process.env.USERPROFILE}/.local/bin/omnillm.exe`
  : `${homedir()}/.local/bin/omnillm`

const server = run("go-server", "31", binaryPath, [
  "start",
  "--port",
  serverPort,
  "--host",
  host,
  "--verbose",
  "--enable-config-edit",
])

// Wait for backend to be ready
// eslint-disable-next-line @typescript-eslint/no-floating-promises
waitForPort(Number(serverPort))

// Start frontend
const frontend = run("frontend", "32", bunExe, [
  "node_modules/.bin/vite",
  "--config",
  "frontend/vite.config.ts",
  "--port",
  frontendPort,
])

function cleanup() {
  server.kill()
  frontend.kill()
  process.exit(0)
}

process.on("SIGINT", cleanup)
process.on("SIGTERM", cleanup)

// Keep the process alive
await Promise.all([server.exited, frontend.exited])
