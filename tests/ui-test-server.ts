import { spawn, type ChildProcessWithoutNullStreams } from "node:child_process"
import { once } from "node:events"
import { mkdtemp, rm } from "node:fs/promises"
import { createServer } from "node:net"
import os from "node:os"
import path from "node:path"

interface UITestServer {
  baseUrl: string
  port: number
  tempHome: string
  stop(): Promise<void>
}

export async function startUITestServer(
  startupTimeoutMs: number = 15_000,
): Promise<UITestServer> {
  const port = await getAvailablePort()
  const tempHome = await mkdtemp(path.join(os.tmpdir(), "omnillm-ui-tests-"))
  const baseUrl = `http://127.0.0.1:${port}`

  const child = spawn(
    process.execPath,
    ["run", "./src/main.ts", "start", "--port", String(port)],
    {
      cwd: process.cwd(),
      env: {
        ...process.env,
        HOME: tempHome,
        USERPROFILE: tempHome,
      },
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: true,
    },
  )

  const stdout: Array<string> = []
  const stderr: Array<string> = []

  child.stdout.setEncoding("utf8")
  child.stderr.setEncoding("utf8")
  child.stdout.on("data", (chunk: string) => {
    stdout.push(chunk)
  })
  child.stderr.on("data", (chunk: string) => {
    stderr.push(chunk)
  })

  try {
    await waitForServer({
      baseUrl,
      child,
      timeoutMs: startupTimeoutMs,
      stdout,
      stderr,
    })
  } catch (error) {
    await terminateChild(child)
    await rm(tempHome, { recursive: true, force: true })
    throw error
  }

  return {
    baseUrl,
    port,
    tempHome,
    async stop() {
      await terminateChild(child)
      await rm(tempHome, { recursive: true, force: true })
    },
  }
}

async function getAvailablePort(): Promise<number> {
  return await new Promise((resolve, reject) => {
    const server = createServer()

    server.on("error", reject)
    server.listen(0, "127.0.0.1", () => {
      const address = server.address()
      if (!address || typeof address === "string") {
        reject(new Error("Failed to allocate a test port"))
        return
      }

      const { port } = address
      server.close((error) => {
        if (error) {
          reject(error)
          return
        }
        resolve(port)
      })
    })
  })
}

async function waitForServer(options: {
  baseUrl: string
  child: ChildProcessWithoutNullStreams
  timeoutMs: number
  stdout: Array<string>
  stderr: Array<string>
}): Promise<void> {
  const { baseUrl, child, timeoutMs, stdout, stderr } = options
  const deadline = Date.now() + timeoutMs

  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(
        [
          `UI test server exited before startup (code ${child.exitCode}).`,
          formatProcessOutput(stdout, stderr),
        ].join("\n"),
      )
    }

    try {
      const response = await fetch(`${baseUrl}/api/admin/info`)
      if (response.ok) {
        return
      }
    } catch {
      // Keep polling until the timeout expires.
    }

    await delay(200)
  }

  throw new Error(
    [
      `Timed out waiting for UI test server at ${baseUrl}.`,
      formatProcessOutput(stdout, stderr),
    ].join("\n"),
  )
}

async function terminateChild(
  child: ChildProcessWithoutNullStreams,
): Promise<void> {
  if (child.exitCode !== null) {
    return
  }

  child.kill()

  const closed = once(child, "close").then(() => undefined)
  const graceful = await Promise.race([closed, delay(5_000)])
  if (graceful === undefined) {
    return
  }

  child.kill("SIGKILL")
  await once(child, "close").catch(() => undefined)
}

function delay(ms: number): Promise<symbol> {
  return new Promise((resolve) => {
    setTimeout(() => resolve(Symbol("timeout")), ms)
  })
}

function formatProcessOutput(
  stdout: Array<string>,
  stderr: Array<string>,
): string {
  const stdoutText = stdout.join("").trim()
  const stderrText = stderr.join("").trim()

  const sections = []
  if (stdoutText) {
    sections.push(`stdout:\n${stdoutText}`)
  }
  if (stderrText) {
    sections.push(`stderr:\n${stderrText}`)
  }

  return sections.length > 0 ? sections.join("\n\n") : "No process output."
}
