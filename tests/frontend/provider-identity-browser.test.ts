import { expect, test } from "bun:test"
import { mkdtemp, rm } from "node:fs/promises"
import { join, resolve } from "node:path"

const browserTest = process.env.OMNILLM_UI_BROWSER_TEST === "1" ? test : test.skip

browserTest("provider ID editor confirms, handles errors, and reloads authoritative state", async () => {
  const chromePath = Bun.which("google-chrome")
  if (!chromePath) throw new Error("google-chrome is unavailable")
  const root = resolve(import.meta.dir, "../../pages/admin")
  if (!(await Bun.file(join(root, "index.html")).exists())) {
    throw new Error("build the frontend before running the browser test")
  }
  const profile = await mkdtemp("/tmp/opencode/omnillm-identity-browser-")
  let providerId = "old-provider"
  let providerName = "Production"
  let providerAlias = "friendly"
  let patches = 0
  let providerReads = 0
  const server = Bun.serve({
    port: 0,
    hostname: "127.0.0.1",
    async fetch(request) {
      const { pathname } = new URL(request.url)
      if (pathname === "/api/admin/info") return Response.json({ version: "test", port: 1, authRequired: false })
      if (pathname === "/api/admin/providers" && request.method === "GET") {
        providerReads++
        return Response.json([{
          id: providerId, type: "alibaba", name: providerName, alias: providerAlias,
          subtitle: providerAlias, isActive: false, authStatus: "unauthenticated",
        }])
      }
      if (pathname === "/api/admin/status") return Response.json({ activeProvider: null, modelCount: 0, authFlow: null })
      if (pathname === "/api/admin/providers/priorities") return Response.json({ priorities: {} })
      if (pathname === "/api/admin/providers/old-provider/identity" && request.method === "PATCH") {
        patches++
        const body = await request.json() as { new_id: string }
        if (patches === 1) return Response.json({ error: "Provider is busy. Please retry." }, { status: 409 })
        providerId = body.new_id
        return Response.json({ success: true, id: providerId })
      }
      if (pathname === "/api/admin/providers/new-provider/name" && request.method === "PATCH") {
        const body = await request.json() as { name?: string; alias?: string }
        providerName = body.name ?? providerName
        providerAlias = body.alias ?? providerAlias
        return Response.json({ success: true, name: providerName, alias: providerAlias, subtitle: providerAlias })
      }
      if (pathname === "/admin/" || pathname === "/admin") return new Response(Bun.file(join(root, "index.html")))
      if (pathname.startsWith("/admin/assets/")) {
        const name = pathname.slice("/admin/assets/".length)
        if (name.includes("/") || name.includes("..")) return new Response(null, { status: 404 })
        return new Response(Bun.file(join(root, "assets", name)))
      }
      return Response.json({ error: "not found" }, { status: 404 })
    },
  })
  const probe = Bun.serve({ port: 0, hostname: "127.0.0.1", fetch: () => new Response() })
  const debugPort = probe.port
  probe.stop(true)
  const chrome = Bun.spawn([
    chromePath, "--headless", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
    "--no-first-run", "--disable-extensions", "--disable-background-networking", "--no-proxy-server",
    `--remote-debugging-port=${debugPort}`, `--user-data-dir=${profile}`,
    `http://127.0.0.1:${server.port}/admin/#providers`,
  ], { stdout: "ignore", stderr: "ignore" })
  let socket: WebSocket | undefined
  try {
    let target: { webSocketDebuggerUrl?: string } | undefined
    for (let attempt = 0; attempt < 120; attempt++) {
      try {
        const response = await fetch(`http://127.0.0.1:${debugPort}/json/list`)
        const tabs = await response.json() as Array<{ type: string; url: string; webSocketDebuggerUrl?: string }>
        target = tabs.find((tab) => tab.type === "page" && tab.url.includes(`/admin/`))
        if (target?.webSocketDebuggerUrl) break
      } catch {
        // Chrome may still be starting.
      }
      await Bun.sleep(50)
    }
    if (!target?.webSocketDebuggerUrl) throw new Error("Chrome debugging target did not start")
    socket = new WebSocket(target.webSocketDebuggerUrl)
    await new Promise<void>((resolveOpen, reject) => {
      socket!.onopen = () => resolveOpen()
      socket!.onerror = () => reject(new Error("Chrome debugging connection failed"))
    })
    let nextId = 0
    const pending = new Map<number, { resolve: (value: any) => void; reject: (error: Error) => void }>()
    socket.onmessage = (event) => {
      const message = JSON.parse(String(event.data)) as { id?: number; result?: any; error?: { message: string } }
      if (message.id === undefined) return
      const entry = pending.get(message.id)
      if (!entry) return
      pending.delete(message.id)
      if (message.error) entry.reject(new Error(message.error.message))
      else entry.resolve(message.result)
    }
    const evaluate = async (expression: string) => {
      const id = ++nextId
      const result = await new Promise<any>((resolveResult, reject) => {
        pending.set(id, { resolve: resolveResult, reject })
        socket!.send(JSON.stringify({ id, method: "Runtime.evaluate", params: { expression, returnByValue: true, awaitPromise: true } }))
      })
      if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails))
      return result.result.value
    }
    const waitFor = async (expression: string) => {
      for (let attempt = 0; attempt < 160; attempt++) {
        if (await evaluate(expression)) return
        await Bun.sleep(50)
      }
      throw new Error(`Timed out waiting for ${expression}`)
    }
    const click = async (selector: string) => {
      const success = await evaluate(`(() => { const element = document.querySelector(${JSON.stringify(selector)}); if (!element) return false; element.click(); return true })()`)
      if (!success) throw new Error(`Missing element ${selector}`)
    }
    const setInput = async (value: string, selector = 'input[aria-label="Provider instance ID"]') => {
      const success = await evaluate(`(() => { const input = document.querySelector(${JSON.stringify(selector)}); if (!input) return false; Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(input, ${JSON.stringify(value)}); input.dispatchEvent(new Event('input', { bubbles: true })); return true })()`)
      if (!success) throw new Error("Provider ID input not found")
    }
    await waitFor(`!!document.querySelector('button[aria-label="Edit ID"]')`)
    await click('button[aria-label="Edit ID"]')
    await setInput("bad/id")
    await click('form button[type="submit"]')
    await waitFor(`!!document.querySelector('[role="alert"]')`)
    expect(patches).toBe(0)
    await click('form button[type="button"]')
    await waitFor(`!document.querySelector('input[aria-label="Provider instance ID"]')`)
    expect(providerId).toBe("old-provider")

    await click('button[aria-label="Edit ID"]')
    await setInput("new-provider")
    await click('form button[type="submit"]')
    await waitFor(`!!document.querySelector('[role="dialog"]')`)
    const warning = await evaluate(`document.querySelector('[role="dialog"]').textContent`)
    expect(warning).toContain("external clients")
    expect(warning).toContain("saved model selections")
    await click('[role="dialog"] .btn-ghost')
    await waitFor(`!document.querySelector('[role="dialog"]')`)
    expect(patches).toBe(0)

    await click('form button[type="submit"]')
    await waitFor(`!!document.querySelector('[role="dialog"]')`)
    await click('[role="dialog"] .btn-primary')
    await waitFor(`document.body.textContent.includes('Provider is busy')`)
    expect(providerId).toBe("old-provider")
    expect(patches).toBe(1)

    await click('form button[type="submit"]')
    await waitFor(`!!document.querySelector('[role="dialog"]')`)
    await click('[role="dialog"] .btn-primary')
    await waitFor(`document.body.textContent.includes('new-provider') && !document.querySelector('input[aria-label="Provider instance ID"]')`)
    expect(providerId).toBe("new-provider")
    expect(patches).toBe(2)
    expect(providerReads).toBeGreaterThan(1)
    expect(await evaluate(`document.body.textContent.includes('Production') && document.body.textContent.includes('friendly')`)).toBe(true)

    await click('button[title="Rename provider"]')
    await waitFor(`!!document.querySelector('input[value="Production"]')`)
    await setInput("Renamed provider", 'input[value="Production"]')
    await waitFor(`[...document.querySelectorAll('input')].some((input) => input.value === 'Renamed provider')`)
    await evaluate(`[...document.querySelectorAll('input')].find((input) => input.value === 'Renamed provider').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))`)
    await waitFor(`document.body.textContent.includes('Renamed provider')`)
    await waitFor(`fetch('/api/admin/providers').then((r) => r.json()).then((p) => p[0].name === 'Renamed provider')`)
    expect(providerName).toBe("Renamed provider")
    await click('button[title="Edit alias"]')
    await waitFor(`!!document.querySelector('input[value="friendly"]')`)
    await setInput("renamed-alias", 'input[value="friendly"]')
    await waitFor(`[...document.querySelectorAll('input')].some((input) => input.value === 'renamed-alias')`)
    await evaluate(`[...document.querySelectorAll('input')].find((input) => input.value === 'renamed-alias').dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))`)
    await waitFor(`document.body.textContent.includes('renamed-alias')`)
    await waitFor(`fetch('/api/admin/providers').then((r) => r.json()).then((p) => p[0].alias === 'renamed-alias')`)
    expect(providerAlias).toBe("renamed-alias")
  } finally {
    socket?.close()
    chrome.kill()
    await chrome.exited
    server.stop(true)
    await rm(profile, { recursive: true, force: true })
  }
}, 30_000)
