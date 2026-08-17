#!/usr/bin/env bun

import {
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs"
import { createServer } from "node:net"
import { basename, dirname, isAbsolute, join, resolve, sep } from "node:path"
import process from "node:process"

const HOST = "127.0.0.1"
export const MINIMUM_SEQUENTIAL_TOOL_CALLS = 5
const INBOUND_API_KEY = "omnillm-live-matrix-inbound"
const DEFAULT_MANIFEST = "scripts/live-model-matrix.json"
const DEFAULT_REPORT_DIR = ".tmp-live-tests/reports"
const DEFAULT_BUDGETS = {
  startupMs: 30_000,
  requestMs: 90_000,
  suiteMs: 600_000,
} as const
const SHAPES = ["chat", "messages", "responses"] as const
let suiteAbortSignal: AbortSignal | undefined
let exactCacheConfigurationFailure: string | undefined
const CAPABILITIES = [
  "tools",
  "parallelTools",
  "largeResults",
  "longStream",
  "cancellation",
  "promptCaching",
  "exactResponseCaching",
] as const
const SMOKE_SCENARIOS = [
  "model_availability",
  "plain",
  "stream",
  "tool_replay",
] as const
const EXTENDED_SCENARIOS = [
  "repeated_tools",
  "parallel_tools",
  "large_result",
  "long_stream",
  "cancellation",
  "prompt_caching",
  "exact_response_caching",
] as const

type Shape = (typeof SHAPES)[number]
type Mode = "smoke" | "extended"
type Outcome = "pass" | "fail" | "skipped" | "not_applicable"
type Scenario =
  | (typeof SMOKE_SCENARIOS)[number]
  | (typeof EXTENDED_SCENARIOS)[number]
type Support = true | { notApplicable: string }
type CredentialReference =
  | { env: string }
  | { tokenBundleEnv: string; target: string }
type Capability = (typeof CAPABILITIES)[number]

export type ManifestRow = {
  id: string
  provider: string
  model: string
  shapes: Partial<Record<Shape, Support>>
  credentials: Array<CredentialReference>
  capabilities: Record<Capability, Support>
  scenarioOverrides?: Partial<Record<Scenario, Support>>
  provision?: {
    path: string
    body: Record<string, unknown>
    activate?: boolean
  }
}

type Budgets = {
  startupMs: number
  requestMs: number
  suiteMs: number
}

type Manifest = {
  version: 1
  budgets?: Partial<Budgets>
  rows: Array<ManifestRow>
}

type MatrixResult = {
  row: string
  provider: string
  model: string
  shape: Shape
  scenario: Scenario
  mode: Mode
  outcome: Outcome
  durationMs: number
  reason?: string
}

type MatrixReport = {
  version: 1
  mode: Mode
  startedAt: string
  finishedAt: string
  manifest: string
  gateway?: { host: string; port: number }
  results: Array<MatrixResult>
  summary: Record<Outcome, number>
}

type Gateway = {
  baseUrl: string
  host: string
  port: number
  process: ReturnType<typeof Bun.spawn>
}

type ToolCall = { id: string; name: string }
type ToolTurn = { payload: unknown; calls: Array<ToolCall> }
type RequestKind = "plain" | "stream" | "long_stream" | "tool"
type StreamStats = { body: string; chunks: number; dataFrames: number }
type JSONResponse = { payload: unknown; headers: Headers }

const SCENARIO_CAPABILITIES: Partial<Record<Scenario, Capability>> = {
  tool_replay: "tools",
  repeated_tools: "tools",
  parallel_tools: "parallelTools",
  large_result: "largeResults",
  long_stream: "longStream",
  cancellation: "cancellation",
  prompt_caching: "promptCaching",
  exact_response_caching: "exactResponseCaching",
}

export function sanitizeFailure(
  error: unknown,
  secrets: ReadonlyArray<string> = [],
): string {
  let message = error instanceof Error ? error.message : String(error)
  for (const secret of secrets) {
    if (secret.length >= 4) message = message.replaceAll(secret, "[REDACTED]")
  }
  message = message
    .replaceAll(
      /(authorization|x-api-key|api[_-]?key|token|secret)\s*[:=]\s*[^\s,;]+/gi,
      "$1=[REDACTED]",
    )
    .replaceAll(/Bearer\s+[^\s,;]+/gi, "Bearer [REDACTED]")
    .replaceAll(/[\r\n\t]+/g, " ")
    .replaceAll(/\s+/g, " ")
    .trim()
  return message.slice(0, 500) || "Unspecified failure"
}

function object(value: unknown, label: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`${label} must be an object`)
  }
  return value as Record<string, unknown>
}

function nonEmptyString(value: unknown, label: string): string {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`${label} must be a non-empty string`)
  }
  return value
}

function rejectUnknownKeys(
  value: Record<string, unknown>,
  allowed: ReadonlyArray<string>,
  label: string,
): void {
  const unknown = Object.keys(value).filter((key) => !allowed.includes(key))
  if (unknown.length > 0)
    throw new Error(`${label} contains unknown field: ${unknown[0]}`)
}

function environmentName(value: unknown, label: string): string {
  const name = nonEmptyString(value, label)
  if (!/^[A-Z][A-Z0-9_]*$/.test(name))
    throw new Error(`${label} must be an environment variable name`)
  return name
}

function support(value: unknown, label: string): Support | undefined {
  if (value === undefined) return undefined
  if (value === true) return true
  const entry = object(value, label)
  rejectUnknownKeys(entry, ["notApplicable"], label)
  return {
    notApplicable: nonEmptyString(
      entry.notApplicable,
      `${label}.notApplicable`,
    ),
  }
}

export function parseManifest(value: unknown): Manifest {
  const root = object(value, "manifest")
  rejectUnknownKeys(root, ["$schema", "version", "budgets", "rows"], "manifest")
  if (root.version !== 1) throw new Error("manifest.version must be 1")
  if (!Array.isArray(root.rows))
    throw new Error("manifest.rows must be an array")

  const ids = new Set<string>()
  const rows = root.rows.map((rawRow, index): ManifestRow => {
    const row = object(rawRow, `rows[${index}]`)
    rejectUnknownKeys(
      row,
      [
        "id",
        "provider",
        "model",
        "shapes",
        "credentials",
        "capabilities",
        "scenarioOverrides",
        "provision",
      ],
      `rows[${index}]`,
    )
    const id = nonEmptyString(row.id, `rows[${index}].id`)
    if (ids.has(id)) throw new Error(`duplicate row id: ${id}`)
    ids.add(id)

    const rawShapes = object(row.shapes, `${id}.shapes`)
    rejectUnknownKeys(rawShapes, SHAPES, `${id}.shapes`)
    const shapes: ManifestRow["shapes"] = {}
    for (const shape of SHAPES)
      shapes[shape] = support(rawShapes[shape], `${id}.shapes.${shape}`)

    if (!Array.isArray(row.credentials))
      throw new Error(`${id}.credentials must be an array`)
    const credentials = row.credentials.map(
      (rawCredential, credentialIndex): CredentialReference => {
        const credential = object(
          rawCredential,
          `${id}.credentials[${credentialIndex}]`,
        )
        const hasEnv = credential.env !== undefined
        const hasBundle =
          credential.tokenBundleEnv !== undefined
          || credential.target !== undefined
        if (hasEnv === hasBundle)
          throw new Error(
            `${id}.credentials[${credentialIndex}] must declare exactly one credential reference`,
          )
        if (hasEnv) {
          rejectUnknownKeys(
            credential,
            ["env"],
            `${id}.credentials[${credentialIndex}]`,
          )
          return {
            env: environmentName(
              credential.env,
              `${id}.credentials[${credentialIndex}].env`,
            ),
          }
        }
        rejectUnknownKeys(
          credential,
          ["tokenBundleEnv", "target"],
          `${id}.credentials[${credentialIndex}]`,
        )
        return {
          tokenBundleEnv: environmentName(
            credential.tokenBundleEnv,
            `${id}.credentials[${credentialIndex}].tokenBundleEnv`,
          ),
          target: nonEmptyString(
            credential.target,
            `${id}.credentials[${credentialIndex}].target`,
          ),
        }
      },
    )

    const rawCapabilities = object(row.capabilities, `${id}.capabilities`)
    rejectUnknownKeys(rawCapabilities, CAPABILITIES, `${id}.capabilities`)
    const capabilities = Object.fromEntries(
      CAPABILITIES.map((name) => {
        const parsed = support(
          rawCapabilities[name],
          `${id}.capabilities.${name}`,
        )
        if (parsed === undefined)
          throw new Error(`${id}.capabilities.${name} is required`)
        return [name, parsed]
      }),
    ) as Record<Capability, Support>

    let scenarioOverrides: ManifestRow["scenarioOverrides"]
    if (row.scenarioOverrides !== undefined) {
      const rawOverrides = object(
        row.scenarioOverrides,
        `${id}.scenarioOverrides`,
      )
      const scenarios = [...SMOKE_SCENARIOS, ...EXTENDED_SCENARIOS]
      rejectUnknownKeys(rawOverrides, scenarios, `${id}.scenarioOverrides`)
      scenarioOverrides = {}
      for (const scenario of scenarios) {
        const parsed = support(
          rawOverrides[scenario],
          `${id}.scenarioOverrides.${scenario}`,
        )
        if (parsed !== undefined) scenarioOverrides[scenario] = parsed
      }
    }

    let provision: ManifestRow["provision"]
    if (row.provision !== undefined) {
      const rawProvision = object(row.provision, `${id}.provision`)
      rejectUnknownKeys(
        rawProvision,
        ["path", "body", "activate"],
        `${id}.provision`,
      )
      const path = nonEmptyString(rawProvision.path, `${id}.provision.path`)
      if (!path.startsWith("/api/admin/"))
        throw new Error(`${id}.provision.path must start with /api/admin/`)
      provision = {
        path,
        body: object(rawProvision.body, `${id}.provision.body`),
        activate:
          rawProvision.activate === undefined || rawProvision.activate === true,
      }
    }

    return {
      id,
      provider: nonEmptyString(row.provider, `${id}.provider`),
      model: nonEmptyString(row.model, `${id}.model`),
      shapes,
      credentials,
      capabilities,
      ...(scenarioOverrides && Object.keys(scenarioOverrides).length > 0 ?
        { scenarioOverrides }
      : {}),
      ...(provision ? { provision } : {}),
    }
  })

  const parsedBudgets =
    root.budgets === undefined ? undefined : object(root.budgets, "budgets")
  if (parsedBudgets)
    rejectUnknownKeys(
      parsedBudgets,
      ["startupMs", "requestMs", "suiteMs"],
      "budgets",
    )
  const budgets =
    parsedBudgets ?
      (Object.fromEntries(
        (["startupMs", "requestMs", "suiteMs"] as const).flatMap((name) => {
          const parsed = positiveBudget(parsedBudgets[name], `budgets.${name}`)
          return parsed === undefined ? [] : [[name, parsed]]
        }),
      ) as Partial<Budgets>)
    : undefined
  return {
    version: 1,
    rows,
    ...(budgets ? { budgets } : {}),
  }
}

function positiveBudget(value: unknown, label: string): number | undefined {
  if (value === undefined) return undefined
  const maximum = label.endsWith("suiteMs") ? 3_600_000 : 300_000
  if (
    !Number.isInteger(value)
    || (value as number) < 1_000
    || (value as number) > maximum
  ) {
    throw new Error(
      `${label} must be an integer between 1000ms and ${maximum}ms`,
    )
  }
  return value as number
}

function modeFromEnvironment(): Mode {
  const value = process.env.OMNILLM_LIVE_MATRIX_MODE ?? "smoke"
  if (value !== "smoke" && value !== "extended") {
    throw new Error("OMNILLM_LIVE_MATRIX_MODE must be smoke or extended")
  }
  return value
}

function manifestPath(): string {
  return resolve(process.env.OMNILLM_LIVE_MATRIX_MANIFEST ?? DEFAULT_MANIFEST)
}

function credentialEnvironmentName(reference: CredentialReference): string {
  return "env" in reference ? reference.env : reference.tokenBundleEnv
}

function missingCredentials(row: ManifestRow): Array<string> {
  return row.credentials
    .map((reference) => credentialEnvironmentName(reference))
    .filter((name) => !process.env[name]?.trim())
}

function credentialValues(rows: Array<ManifestRow>): Array<string> {
  return rows.flatMap((row) =>
    row.credentials
      .map(
        (reference) => process.env[credentialEnvironmentName(reference)] ?? "",
      )
      .filter(Boolean),
  )
}

function safeTarget(home: string, target: string): string {
  if (isAbsolute(target))
    throw new Error("token bundle target must be relative to temporary HOME")
  const destination = resolve(home, target)
  if (!destination.startsWith(`${resolve(home)}${sep}`)) {
    throw new Error("token bundle target escapes temporary HOME")
  }
  return destination
}

function copyTokenBundles(rows: Array<ManifestRow>, home: string): void {
  const copied = new Set<string>()
  for (const row of rows) {
    for (const reference of row.credentials) {
      if (!("tokenBundleEnv" in reference)) continue
      const source = process.env[reference.tokenBundleEnv]
      if (!source) continue
      if (!existsSync(source))
        throw new Error(
          `${reference.tokenBundleEnv} does not reference a readable file`,
        )
      const destination = safeTarget(home, reference.target)
      const pair = `${resolve(source)}\0${destination}`
      if (copied.has(pair)) continue
      copied.add(pair)
      mkdirSync(dirname(destination), { recursive: true, mode: 0o700 })
      copyFileSync(source, destination)
    }
  }
}

function resolveEnvironmentReferences(value: unknown): unknown {
  if (Array.isArray(value))
    return value.map((entry) => resolveEnvironmentReferences(entry))
  if (!value || typeof value !== "object") return value
  const entry = value as Record<string, unknown>
  if (Object.keys(entry).length === 1 && typeof entry.$env === "string") {
    const resolved = process.env[entry.$env]
    if (!resolved)
      throw new Error(`required credential ${entry.$env} is unavailable`)
    return resolved
  }
  return Object.fromEntries(
    Object.entries(entry).map(([key, child]) => [
      key,
      resolveEnvironmentReferences(child),
    ]),
  )
}

async function allocatePort(): Promise<number> {
  const server = createServer()
  return await new Promise((resolvePort, reject) => {
    server.once("error", reject)
    server.listen(0, HOST, () => {
      const address = server.address()
      if (!address || typeof address === "string") {
        server.close()
        reject(new Error("failed to allocate loopback port"))
        return
      }
      const port = address.port
      server.close((error) => (error ? reject(error) : resolvePort(port)))
    })
  })
}

async function runCommand(
  command: string,
  args: Array<string>,
  env?: Record<string, string>,
): Promise<void> {
  const child = Bun.spawn([command, ...args], {
    cwd: process.cwd(),
    env: env ?? process.env,
    stdin: "ignore",
    stdout: "inherit",
    stderr: "inherit",
  })
  const exitCode = await child.exited
  if (exitCode !== 0)
    throw new Error(`${command} failed with exit code ${exitCode}`)
}

async function fetchBounded(
  url: string,
  init: RequestInit,
  timeoutMs: number,
  suiteSignal?: AbortSignal,
): Promise<Response> {
  const timeoutSignal = AbortSignal.timeout(timeoutMs)
  const parentSignal = suiteSignal ?? suiteAbortSignal
  const signal =
    parentSignal ?
      AbortSignal.any([timeoutSignal, parentSignal])
    : timeoutSignal
  return await fetch(url, { ...init, signal })
}

function authHeaders(): Record<string, string> {
  return {
    Authorization: `Bearer ${INBOUND_API_KEY}`,
    "Content-Type": "application/json",
    "x-api-key": INBOUND_API_KEY,
  }
}

async function waitUntilReady(
  baseUrl: string,
  timeoutMs: number,
): Promise<void> {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    try {
      const response = await fetchBounded(
        `${baseUrl}/api/admin/info`,
        {},
        1_000,
      )
      if (response.ok) return
    } catch {
      // The child is still starting.
    }
    await Bun.sleep(100)
  }
  throw new Error(`gateway startup exceeded ${timeoutMs}ms`)
}

async function launchGateway(
  binaryPath: string,
  home: string,
  port: number,
  startupMs: number,
): Promise<Gateway> {
  const baseUrl = `http://${HOST}:${port}`
  const env: Record<string, string> = {
    HOME: home,
    USERPROFILE: home,
    XDG_CONFIG_HOME: join(home, ".config"),
    TMPDIR: join(home, "tmp"),
  }
  for (const name of ["PATH", "SSL_CERT_FILE", "SSL_CERT_DIR", "TZ"] as const) {
    const value = process.env[name]
    if (value) env[name] = value
  }
  mkdirSync(env.TMPDIR, { recursive: true, mode: 0o700 })
  const child = Bun.spawn(
    [
      binaryPath,
      "start",
      "--host",
      HOST,
      "--port",
      String(port),
      "--api-key",
      INBOUND_API_KEY,
      "--response-cache-redis-url",
      process.env.OMNILLM_TEST_REDIS_URL ?? "redis://127.0.0.1:6379/0",
      "--response-cache-redis-prefix",
      `omnillm-live-${basename(home)}`,
    ],
    {
      cwd: process.cwd(),
      env,
      stdin: "ignore",
      stdout: "ignore",
      stderr: "ignore",
    },
  )
  try {
    await waitUntilReady(baseUrl, startupMs)
  } catch (error) {
    child.kill()
    await child.exited
    throw error
  }
  return { baseUrl, host: HOST, port, process: child }
}

async function stopGateway(gateway: Gateway): Promise<void> {
  gateway.process.kill("SIGTERM")
  const exited = gateway.process.exited.then(() => undefined)
  const timedOut = Bun.sleep(5_000).then(() => "timeout" as const)
  if ((await Promise.race([exited, timedOut])) === "timeout") {
    gateway.process.kill("SIGKILL")
    await gateway.process.exited
  }
}

async function requestJSONWithHeaders(
  baseUrl: string,
  path: string,
  body: unknown,
  timeoutMs: number,
): Promise<JSONResponse> {
  const response = await fetchBounded(
    `${baseUrl}${path}`,
    { method: "POST", headers: authHeaders(), body: JSON.stringify(body) },
    timeoutMs,
  )
  const text = await response.text()
  if (!response.ok) throw new Error(`${path} returned HTTP ${response.status}`)
  try {
    return { payload: JSON.parse(text) as unknown, headers: response.headers }
  } catch {
    throw new Error(`${path} returned invalid JSON`)
  }
}

async function requestJSON(
  baseUrl: string,
  path: string,
  body: unknown,
  timeoutMs: number,
): Promise<unknown> {
  return (await requestJSONWithHeaders(baseUrl, path, body, timeoutMs)).payload
}

async function setExactResponseCacheEnabled(
  baseUrl: string,
  timeoutMs: number,
  enabled: boolean,
): Promise<string | undefined> {
  const path = "/api/admin/settings/response-cache"
  try {
    const response = await fetchBounded(
      `${baseUrl}${path}`,
      {
        method: "PUT",
        headers: authHeaders(),
        body: JSON.stringify({ enabled, ttl_seconds: 60 }),
      },
      timeoutMs,
    )
    if (!response.ok) return `${path} returned HTTP ${response.status}`
    return undefined
  } catch (error) {
    return sanitizeFailure(error)
  }
}

export function needsExactResponseCache(
  mode: Mode,
  rows: ReadonlyArray<ManifestRow>,
): boolean {
  if (mode !== "extended") return false
  return rows.some(
    (row) =>
      row.capabilities.exactResponseCaching === true
      && SHAPES.some(
        (shape) =>
          row.shapes[shape] === true
          && (row.scenarioOverrides?.exact_response_caching === undefined
            || row.scenarioOverrides.exact_response_caching === true),
      ),
  )
}

async function provisionRow(
  row: ManifestRow,
  baseUrl: string,
  timeoutMs: number,
): Promise<void> {
  if (!row.provision) return
  const payload = object(
    await requestJSON(
      baseUrl,
      row.provision.path,
      resolveEnvironmentReferences(row.provision.body),
      timeoutMs,
    ),
    `${row.id} provision response`,
  )
  if (payload.success !== true)
    throw new Error(`${row.id} provider provisioning was not successful`)
  if (row.provision.activate === false) return
  const provider = object(payload.provider, `${row.id} provision provider`)
  const providerID = nonEmptyString(
    provider.id,
    `${row.id} provision provider.id`,
  )
  await requestJSON(
    baseUrl,
    `/api/admin/providers/${encodeURIComponent(providerID)}/activate`,
    {},
    timeoutMs,
  )
}

function requestFor(
  shape: Shape,
  model: string,
  kind: RequestKind,
  promptOverride?: string,
): unknown {
  let defaultPrompt =
    "Reply with a short confirmation that the compatibility request succeeded."
  if (kind === "tool")
    defaultPrompt =
      promptOverride ?? "Call the get_weather tool exactly once for Paris."
  else if (kind === "long_stream")
    defaultPrompt =
      "Write at least 40 numbered short lines, one per line, to exercise a sustained streaming response."
  const prompt = promptOverride ?? defaultPrompt
  const tool = {
    name: "get_weather",
    description: "Get current weather for a city",
    input_schema: {
      type: "object",
      properties: { city: { type: "string" } },
      required: ["city"],
    },
  }
  if (shape === "messages") {
    return {
      model,
      max_tokens: kind === "long_stream" ? 2048 : 512,
      stream: kind === "stream" || kind === "long_stream",
      messages: [{ role: "user", content: prompt }],
      ...(kind === "tool" ?
        { tools: [tool], tool_choice: { type: "tool", name: tool.name } }
      : {}),
    }
  }
  const functionTool = {
    type: "function",
    function: {
      name: tool.name,
      description: tool.description,
      parameters: tool.input_schema,
    },
  }
  if (shape === "responses") {
    return {
      model,
      max_output_tokens: kind === "long_stream" ? 2048 : 512,
      stream: kind === "stream" || kind === "long_stream",
      input: prompt,
      ...(kind === "tool" ?
        {
          tools: [
            {
              type: "function",
              name: tool.name,
              description: tool.description,
              parameters: tool.input_schema,
              strict: false,
            },
          ],
          tool_choice: { type: "function", name: tool.name },
        }
      : {}),
    }
  }
  return {
    model,
    max_tokens: 512,
    stream: kind === "stream",
    messages: [{ role: "user", content: prompt }],
    ...(kind === "tool" ?
      {
        tools: [functionTool],
        tool_choice: { type: "function", function: { name: tool.name } },
      }
    : {}),
  }
}

function endpoint(shape: Shape): string {
  if (shape === "messages") return "/v1/messages"
  if (shape === "responses") return "/v1/responses"
  return "/v1/chat/completions"
}

function requireText(shape: Shape, payload: unknown): void {
  const root = object(payload, `${shape} response`)
  let text = ""
  if (shape === "chat") {
    const choices = root.choices
    if (Array.isArray(choices) && choices.length > 0) {
      const choice = object(choices[0], "chat choices[0]")
      const message = object(choice.message, "chat choices[0].message")
      text = typeof message.content === "string" ? message.content : ""
    }
  } else if (shape === "messages") {
    if (Array.isArray(root.content)) {
      text = root.content
        .map((item) => object(item, "messages content item"))
        .filter((item) => item.type === "text")
        .map((item) => (typeof item.text === "string" ? item.text : ""))
        .join("")
    }
  } else if (Array.isArray(root.output)) {
    for (const rawItem of root.output) {
      const item = object(rawItem, "responses output item")
      if (!Array.isArray(item.content)) continue
      for (const rawContent of item.content) {
        const content = object(rawContent, "responses content item")
        if (typeof content.text === "string") text += content.text
      }
    }
  }
  if (text.trim() === "")
    throw new Error(`${shape} response contained no assistant text`)
}

function extractToolTurn(shape: Shape, payload: unknown): ToolTurn {
  const root = object(payload, `${shape} tool response`)
  const calls: Array<ToolCall> = []
  if (shape === "chat") {
    const choices = root.choices
    if (Array.isArray(choices) && choices.length > 0) {
      const message = object(
        object(choices[0], "chat choice").message,
        "chat message",
      )
      if (Array.isArray(message.tool_calls)) {
        for (const rawCall of message.tool_calls) {
          const call = object(rawCall, "chat tool call")
          const fn = object(call.function, "chat tool function")
          calls.push({
            id: nonEmptyString(call.id, "chat tool call id"),
            name: nonEmptyString(fn.name, "chat tool name"),
          })
        }
      }
      return { payload: message, calls }
    }
  } else if (shape === "messages") {
    if (Array.isArray(root.content)) {
      for (const rawCall of root.content) {
        const call = object(rawCall, "messages content")
        if (call.type !== "tool_use") continue
        calls.push({
          id: nonEmptyString(call.id, "messages tool id"),
          name: nonEmptyString(call.name, "messages tool name"),
        })
      }
      return { payload: root.content, calls }
    }
  } else if (Array.isArray(root.output)) {
    for (const rawCall of root.output) {
      const call = object(rawCall, "responses output")
      if (call.type !== "function_call") continue
      calls.push({
        id: nonEmptyString(call.call_id ?? call.id, "responses tool id"),
        name: nonEmptyString(call.name, "responses tool name"),
      })
    }
    return { payload: root.output, calls }
  }
  return { payload: undefined, calls }
}

function repeatedToolPrompt(model: string, result: string): string {
  const cycleMatch = /cycle-(\d+)/.exec(result)
  const cycle = cycleMatch ? Number(cycleMatch[1]) : 1
  const cities = ["Paris", "London", "Tokyo", "Sydney", "Toronto"]
  const city = cities[Math.min(cycle, cities.length - 1)]
  const family = model.toLowerCase().startsWith("grok-") ? "Grok" : "The model"
  return `${family} must call get_weather exactly once for ${city} now. This is required tool cycle ${cycle}; the previous result was ${result}. Do not answer with text.`
}

export function replayRequest(
  shape: Shape,
  model: string,
  turn: ToolTurn,
  result: string,
  forceTools = false,
  previousRequest: unknown = requestFor(shape, model, "tool"),
): unknown {
  const base = object(previousRequest, `${shape} previous request`)
  const followUpPrompt =
    forceTools ? repeatedToolPrompt(model, result) : "Summarize the weather result."
  if (shape === "chat") {
    return {
      ...base,
      tool_choice: forceTools ? base.tool_choice : "none",
      messages: [
        ...(base.messages as Array<unknown>),
        {
          role: "assistant",
          content: null,
          tool_calls: (turn.payload as Record<string, unknown>).tool_calls,
        },
        ...turn.calls.map((call) => ({
          role: "tool",
          tool_call_id: call.id,
          content: result,
        })),
        {
          role: "user",
          content: followUpPrompt,
        },
      ],
    }
  }
  if (shape === "messages") {
    return {
      ...base,
      tool_choice: forceTools ? base.tool_choice : { type: "auto" },
      messages: [
        ...(base.messages as Array<unknown>),
        { role: "assistant", content: turn.payload },
        {
          role: "user",
          content: turn.calls.map((call) => ({
            type: "tool_result",
            tool_use_id: call.id,
            content: result,
          })),
        },
        {
          role: "user",
          content: followUpPrompt,
        },
      ],
    }
  }
  let previousInput: Array<unknown> = []
  if (Array.isArray(base.input)) previousInput = base.input as Array<unknown>
  else if (typeof base.input === "string")
    previousInput = [{ role: "user", content: base.input }]
  return {
    ...base,
    tool_choice: forceTools ? base.tool_choice : "none",
    input: [
      ...previousInput,
      ...(turn.payload as Array<unknown>),
      ...turn.calls.map((call) => ({
        type: "function_call_output",
        call_id: call.id,
        output: result,
      })),
      {
        role: "user",
        content: followUpPrompt,
      },
    ],
  }
}

async function modelAvailability(
  baseUrl: string,
  model: string,
  timeoutMs: number,
): Promise<void> {
  const availabilityTimeoutMs = Math.min(timeoutMs, 30_000)
  const response = await fetchBounded(
    `${baseUrl}/v1/models`,
    { headers: authHeaders() },
    availabilityTimeoutMs,
  )
  if (!response.ok)
    throw new Error(`/v1/models returned HTTP ${response.status}`)
  const payload = object(await response.json(), "models response")
  if (!Array.isArray(payload.data))
    throw new Error("models response did not contain data")
  const present = payload.data.some(
    (entry) => object(entry, "model entry").id === model,
  )
  if (!present)
    throw new Error(`model ${model} was not advertised by the isolated gateway`)
}

async function plain(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
): Promise<void> {
  const payload = await requestJSON(
    baseUrl,
    endpoint(shape),
    requestFor(shape, row.model, "plain"),
    timeoutMs,
  )
  requireText(shape, payload)
}

export async function readStream(response: Response): Promise<StreamStats> {
  if (!response.body) throw new Error("stream response did not contain a body")
  const reader = response.body.getReader()
  let body = ""
  let chunks = 0
  const decoder = new TextDecoder()
  try {
    while (true) {
      const chunk = await reader.read()
      if (chunk.done) break
      chunks += 1
      const decoded = decoder.decode(chunk.value as Uint8Array, { stream: true })
      body += decoded
      if (
        decoded.includes("data: [DONE]")
        || decoded.includes("event: message_stop")
        || decoded.includes("response.completed")
      ) {
        break
      }
    }
  } finally {
    await reader.cancel().catch(() => undefined)
  }
  body += decoder.decode()
  return { body, chunks, dataFrames: body.split("data:").length - 1 }
}

function assertTerminalStream(shape: Shape, stats: StreamStats): void {
  if (
    stats.dataFrames === 0
    || (shape === "messages" && !stats.body.includes("event:"))
  ) {
    throw new Error(
      `${shape} stream did not contain expected server-sent events`,
    )
  }
  let terminal: boolean
  if (shape === "chat") terminal = stats.body.includes("[DONE]")
  else if (shape === "messages") terminal = stats.body.includes("message_stop")
  else
    terminal =
      stats.body.includes("response.completed") || stats.body.includes("[DONE]")
  if (!terminal)
    throw new Error(`${shape} stream did not contain a terminal event`)
}

async function stream(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
  kind: "stream" | "long_stream" = "stream",
): Promise<StreamStats> {
  const response = await fetchBounded(
    `${baseUrl}${endpoint(shape)}`,
    {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(requestFor(shape, row.model, kind)),
    },
    timeoutMs,
  )
  if (!response.ok)
    throw new Error(
      `${endpoint(shape)} stream returned HTTP ${response.status}`,
    )
  const stats = await readStream(response)
  assertTerminalStream(shape, stats)
  return stats
}

async function toolRequest(
  baseUrl: string,
  shape: Shape,
  timeoutMs: number,
  request: unknown,
): Promise<ToolTurn> {
  const payload = await requestJSON(
    baseUrl,
    endpoint(shape),
    request,
    timeoutMs,
  )
  const turn = extractToolTurn(shape, payload)
  if (turn.calls.length === 0)
    throw new Error(`${shape} response did not contain a tool call`)
  return turn
}

async function toolReplay(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
  result = "live-matrix",
  forceTools = false,
): Promise<ToolTurn> {
  const initialRequest = requestFor(shape, row.model, "tool")
  const turn = await toolRequest(baseUrl, shape, timeoutMs, initialRequest)
  const replay = await requestJSON(
    baseUrl,
    endpoint(shape),
    replayRequest(shape, row.model, turn, result, forceTools, initialRequest),
    timeoutMs,
  )
  if (forceTools) {
    const next = extractToolTurn(shape, replay)
    if (next.calls.length === 0)
      throw new Error(`${shape} repeated turn did not contain a tool call`)
    return next
  }
  requireText(shape, replay)
  return turn
}

async function repeatedTools(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
): Promise<void> {
  let request = requestFor(shape, row.model, "tool")
  let turn = await toolRequest(baseUrl, shape, timeoutMs, request)
  for (let cycle = 1; cycle <= MINIMUM_SEQUENTIAL_TOOL_CALLS; cycle += 1) {
    const forceTools = cycle < MINIMUM_SEQUENTIAL_TOOL_CALLS
    request = replayRequest(
      shape,
      row.model,
      turn,
      `cycle-${cycle}`,
      forceTools,
      request,
    )
    const payload = await requestJSON(
      baseUrl,
      endpoint(shape),
      request,
      timeoutMs,
    )
    if (!forceTools) {
      requireText(shape, payload)
      return
    }
    turn = extractToolTurn(shape, payload)
    if (turn.calls.length === 0)
      throw new Error(`${shape} repeated turn did not contain a tool call`)
  }
}

async function parallelTools(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
): Promise<void> {
  const payload = requestFor(shape, row.model, "tool") as Record<
    string,
    unknown
  >
  const prompt =
    "Call get_weather twice in parallel: once for Paris and once for London."
  if (shape === "responses") payload.input = prompt
  else payload.messages = [{ role: "user", content: prompt }]
  delete payload.tool_choice
  const response = await requestJSON(
    baseUrl,
    endpoint(shape),
    payload,
    timeoutMs,
  )
  const turn = extractToolTurn(shape, response)
  if (turn.calls.length < 2)
    throw new Error(`${shape} did not return two parallel tool calls`)
  const replay = await requestJSON(
    baseUrl,
    endpoint(shape),
    replayRequest(shape, row.model, turn, "parallel-result"),
    timeoutMs,
  )
  requireText(shape, replay)
}

async function longStream(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
): Promise<void> {
  const stats = await stream(baseUrl, row, shape, timeoutMs, "long_stream")
  if (stats.chunks < 1 || stats.dataFrames < 1) {
    throw new Error(
      `${shape} long stream did not show continued/chunked activity`,
    )
  }
}

export function cacheReadTokens(shape: Shape, payload: unknown): number {
  const root = object(payload, `${shape} cache response`)
  const usage = object(root.usage, `${shape} usage`)
  let value: unknown
  if (shape === "chat") {
    value = object(usage.prompt_tokens_details, "prompt token details").cached_tokens
  } else if (shape === "messages") {
    value = usage.cache_read_input_tokens
  } else {
    value = object(usage.input_tokens_details, "input token details").cached_tokens
  }
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0) {
    throw new Error(`${shape} response did not contain valid cache-read usage`)
  }
  return value
}

function promptCacheRequests(shape: Shape, model: string): [unknown, unknown] {
  const stable = "Stable prompt-cache prefix. ".repeat(600)
  if (shape === "messages") {
    const base = {
      model,
      max_tokens: 512,
      system: [
        {
          type: "text",
          text: stable,
          cache_control: { type: "ephemeral" },
        },
      ],
    }
    return [
      { ...base, messages: [{ role: "user", content: "Request A" }] },
      { ...base, messages: [{ role: "user", content: "Request B" }] },
    ]
  }
  const key = `live-matrix-${model}`
  if (shape === "responses") {
    const base = { model, max_output_tokens: 512, prompt_cache_key: key }
    return [
      { ...base, input: `${stable}\nRequest A` },
      { ...base, input: `${stable}\nRequest B` },
    ]
  }
  const base = { model, max_tokens: 512, prompt_cache_key: key }
  return [
    { ...base, messages: [{ role: "user", content: `${stable}\nRequest A` }] },
    { ...base, messages: [{ role: "user", content: `${stable}\nRequest B` }] },
  ]
}

async function promptCaching(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
): Promise<void> {
  const [first, second] = promptCacheRequests(shape, row.model)
  const firstResponse = await requestJSON(baseUrl, endpoint(shape), first, timeoutMs)
  requireText(shape, firstResponse)
  const secondResponse = await requestJSON(baseUrl, endpoint(shape), second, timeoutMs)
  requireText(shape, secondResponse)
  if (cacheReadTokens(shape, secondResponse) <= 0) {
    throw new Error(`${shape} second request reported zero cache-read tokens`)
  }
}

async function exactResponseCaching(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  timeoutMs: number,
): Promise<void> {
  if (exactCacheConfigurationFailure) {
    throw new Error(exactCacheConfigurationFailure)
  }
  const request = requestFor(
    shape,
    row.model,
    "plain",
    `Return the exact-response-cache live matrix marker for ${row.id} ${shape}.`,
  )
  const first = await requestJSONWithHeaders(
    baseUrl,
    endpoint(shape),
    request,
    timeoutMs,
  )
  requireText(shape, first.payload)
  if (first.headers.get("X-OmniLLM-Cache") !== "miss") {
    throw new Error(
      `${shape} exact-response population did not report cache miss`,
    )
  }
  const second = await requestJSONWithHeaders(
    baseUrl,
    endpoint(shape),
    request,
    timeoutMs,
  )
  requireText(shape, second.payload)
  if (second.headers.get("X-OmniLLM-Cache") !== "hit") {
    throw new Error(`${shape} exact-response replay did not report cache hit`)
  }
  const firstRoot = object(first.payload, `${shape} population response`)
  const secondRoot = object(second.payload, `${shape} replay response`)
  const firstOutput = JSON.stringify(
    firstRoot.output ?? firstRoot.content ?? firstRoot.choices,
  )
  const secondOutput = JSON.stringify(
    secondRoot.output ?? secondRoot.content ?? secondRoot.choices,
  )
  if (firstRoot.id !== secondRoot.id || firstOutput !== secondOutput) {
    throw new Error(`${shape} exact-response replay changed canonical output`)
  }

  const streamRequest = requestFor(
    shape,
    row.model,
    "stream",
    `Stream the exact-response-cache live matrix marker for ${row.id} ${shape}.`,
  )
  const population = await fetchBounded(
    `${baseUrl}${endpoint(shape)}`,
    {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(streamRequest),
    },
    timeoutMs,
  )
  if (!population.ok)
    throw new Error(`${shape} exact-response stream population returned HTTP ${population.status}`)
  const populationStats = await readStream(population)
  assertTerminalStream(shape, populationStats)
  if (population.headers.get("X-OmniLLM-Cache") !== "miss")
    throw new Error(`${shape} exact-response stream population did not report cache miss`)

  const replay = await fetchBounded(
    `${baseUrl}${endpoint(shape)}`,
    {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(streamRequest),
    },
    timeoutMs,
  )
  if (!replay.ok)
    throw new Error(`${shape} exact-response stream replay returned HTTP ${replay.status}`)
  const replayStats = await readStream(replay)
  assertTerminalStream(shape, replayStats)
  if (replay.headers.get("X-OmniLLM-Cache") !== "hit")
    throw new Error(`${shape} exact-response stream replay did not report cache hit`)

  const terminalMarker =
    shape === "chat" ? "[DONE]"
    : shape === "messages" ? "message_stop"
    : "response.completed"
  if (replayStats.body.split(terminalMarker).length - 1 !== 1)
    throw new Error(`${shape} exact-response stream replay emitted duplicate terminal events`)
}

async function cancellation(
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
): Promise<void> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), 250)
  let response: Response | undefined
  try {
    response = await fetch(`${baseUrl}${endpoint(shape)}`, {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(
        requestFor(
          shape,
          row.model,
          "long_stream",
          "Write an extremely detailed numbered list with at least 500 short lines.",
        ),
      ),
      signal: controller.signal,
    })
    if (!response.ok)
      throw new Error(
        `${endpoint(shape)} cancellation stream returned HTTP ${response.status}`,
      )
    await readStream(response)
    if (!controller.signal.aborted)
      throw new Error(
        `${shape} stream completed before cancellation could be exercised`,
      )
  } catch (error) {
    if (!controller.signal.aborted) throw error
  } finally {
    clearTimeout(timer)
    await response?.body?.cancel().catch(() => undefined)
  }
  await modelAvailability(baseUrl, row.model, 30_000)
}

function scenarioCapability(scenario: Scenario): Capability | undefined {
  return SCENARIO_CAPABILITIES[scenario]
}

async function executeScenario(
  scenario: Scenario,
  baseUrl: string,
  row: ManifestRow,
  shape: Shape,
  requestMs: number,
): Promise<void> {
  const needsCache = scenario === "exact_response_caching"
  const cacheError = await setExactResponseCacheEnabled(baseUrl, requestMs, needsCache)
  if (needsCache) {
    if (cacheError) exactCacheConfigurationFailure = cacheError
    else exactCacheConfigurationFailure = undefined
  } else if (cacheError) {
    throw new Error(cacheError)
  }
  if (scenario === "model_availability") {
    await modelAvailability(baseUrl, row.model, requestMs)
    return
  }
  if (scenario === "plain") {
    await plain(baseUrl, row, shape, requestMs)
    return
  }
  if (scenario === "stream") {
    await stream(baseUrl, row, shape, requestMs)
    return
  }
  if (scenario === "long_stream") {
    await longStream(baseUrl, row, shape, requestMs)
    return
  }
  if (scenario === "tool_replay") {
    await toolReplay(baseUrl, row, shape, requestMs)
    return
  }
  if (scenario === "repeated_tools") {
    await repeatedTools(baseUrl, row, shape, requestMs)
    return
  }
  if (scenario === "parallel_tools") {
    await parallelTools(baseUrl, row, shape, requestMs)
    return
  }
  if (scenario === "large_result") {
    await toolReplay(baseUrl, row, shape, requestMs, "x".repeat(256 * 1024))
    return
  }
  if (scenario === "prompt_caching") {
    await promptCaching(baseUrl, row, shape, requestMs)
    return
  }
  if (scenario === "exact_response_caching") {
    await exactResponseCaching(baseUrl, row, shape, requestMs)
    return
  }
  await cancellation(baseUrl, row, shape)
}

function plannedScenarios(mode: Mode): Array<Scenario> {
  return mode === "extended" ?
      [...SMOKE_SCENARIOS, ...EXTENDED_SCENARIOS]
    : [...SMOKE_SCENARIOS]
}

function preclassifiedResult(
  row: ManifestRow,
  shape: Shape,
  scenario: Scenario,
  mode: Mode,
  missing: ReadonlyArray<string>,
): MatrixResult | undefined {
  const shapeSupport = row.shapes[shape]
  if (shapeSupport !== true) {
    return result(
      row,
      shape,
      scenario,
      mode,
      "not_applicable",
      0,
      shapeSupport?.notApplicable ?? "Shape not declared supported",
    )
  }
  const scenarioSupport = row.scenarioOverrides?.[scenario]
  if (scenarioSupport !== undefined && scenarioSupport !== true) {
    return result(
      row,
      shape,
      scenario,
      mode,
      "not_applicable",
      0,
      scenarioSupport.notApplicable,
    )
  }
  const capability = scenarioCapability(scenario)
  if (capability && row.capabilities[capability] !== true) {
    return result(
      row,
      shape,
      scenario,
      mode,
      "not_applicable",
      0,
      (row.capabilities[capability] as { notApplicable: string }).notApplicable,
    )
  }
  if (missing.length > 0) {
    return result(
      row,
      shape,
      scenario,
      mode,
      "skipped",
      0,
      `Missing credential references: ${missing.join(", ")}`,
    )
  }
  return undefined
}

function result(
  row: ManifestRow,
  shape: Shape,
  scenario: Scenario,
  mode: Mode,
  outcome: Outcome,
  durationMs: number,
  reason?: string,
): MatrixResult {
  return {
    row: row.id,
    provider: row.provider,
    model: row.model,
    shape,
    scenario,
    mode,
    outcome,
    durationMs,
    ...(reason ? { reason } : {}),
  }
}

async function executeRows(
  manifest: Manifest,
  mode: Mode,
  baseUrl: string,
  requestMs: number,
  secrets: Array<string>,
  missingByRow: ReadonlyMap<string, ReadonlyArray<string>>,
  rowFailures: ReadonlyMap<string, string> = new Map(),
): Promise<Array<MatrixResult>> {
  const results: Array<MatrixResult> = []
  for (const row of manifest.rows) {
    const missing = missingByRow.get(row.id) ?? []
    for (const shape of SHAPES) {
      for (const scenario of plannedScenarios(mode)) {
        const preclassified = preclassifiedResult(
          row,
          shape,
          scenario,
          mode,
          missing,
        )
        if (preclassified) {
          results.push(preclassified)
          continue
        }
        const rowFailure = rowFailures.get(row.id)
        if (rowFailure) {
          results.push(
            result(row, shape, scenario, mode, "fail", 0, rowFailure),
          )
          continue
        }
        const started = performance.now()
        try {
          await executeScenario(scenario, baseUrl, row, shape, requestMs)
          results.push(
            result(
              row,
              shape,
              scenario,
              mode,
              "pass",
              Math.round(performance.now() - started),
            ),
          )
        } catch (error) {
          results.push(
            result(
              row,
              shape,
              scenario,
              mode,
              "fail",
              Math.round(performance.now() - started),
              sanitizeFailure(error, secrets),
            ),
          )
        }
      }
    }
  }
  return results
}

function summary(results: Array<MatrixResult>): Record<Outcome, number> {
  const totals: Record<Outcome, number> = {
    pass: 0,
    fail: 0,
    skipped: 0,
    not_applicable: 0,
  }
  for (const entry of results) totals[entry.outcome] += 1
  return totals
}

function printSummary(report: MatrixReport, reportPath: string): void {
  console.log(`Live model compatibility matrix (${report.mode})`)
  for (const entry of report.results) {
    const detail = entry.reason ? ` - ${entry.reason}` : ""
    console.log(
      `${entry.outcome.padEnd(14)} ${entry.row} ${entry.shape} ${entry.scenario}${detail}`,
    )
  }
  const totals = report.summary
  console.log(
    `Summary: ${totals.pass} passed, ${totals.fail} failed, ${totals.skipped} skipped, ${totals.not_applicable} not applicable`,
  )
  console.log(`JSON report: ${reportPath}`)
}

function writeReport(report: MatrixReport): string {
  const reportDir = resolve(
    process.env.OMNILLM_LIVE_MATRIX_REPORT_DIR ?? DEFAULT_REPORT_DIR,
  )
  mkdirSync(reportDir, { recursive: true })
  const path = join(
    reportDir,
    `live-model-matrix-${report.mode}-${Date.now()}.json`,
  )
  writeFileSync(path, `${JSON.stringify(report, null, 2)}\n`, { mode: 0o600 })
  return path
}

export async function main(): Promise<number> {
  // This guard must remain before manifest reads, credential lookups, port allocation,
  // builds, temporary state creation, and all network-capable operations.
  if (process.env.OMNILLM_RUN_LIVE_MATRIX !== "1") {
    console.log(
      "Live model compatibility matrix skipped: set OMNILLM_RUN_LIVE_MATRIX=1 to opt in.",
    )
    return 0
  }

  const startedAt = new Date().toISOString()
  const mode = modeFromEnvironment()
  const sourceManifest = manifestPath()
  const manifest = parseManifest(
    JSON.parse(readFileSync(sourceManifest, "utf8")) as unknown,
  )
  const budgets = { ...DEFAULT_BUDGETS, ...manifest.budgets }
  const secrets = credentialValues(manifest.rows)
  const missingByRow = new Map(
    manifest.rows.map((row) => [row.id, missingCredentials(row)]),
  )
  const runnableRows = manifest.rows.filter(
    (row) => (missingByRow.get(row.id) ?? []).length === 0,
  )
  let results: Array<MatrixResult> = []
  let gateway: Gateway | undefined
  let tempRoot: string | undefined

  try {
    if (runnableRows.length > 0) {
      tempRoot = mkdtempSync(
        join(process.env.TMPDIR ?? "/tmp", "omnillm-live-matrix-"),
      )
      const home = join(tempRoot, "home")
      const binaryDir = join(tempRoot, "bin")
      mkdirSync(home, { recursive: true, mode: 0o700 })
      mkdirSync(binaryDir, { recursive: true, mode: 0o700 })
      copyTokenBundles(runnableRows, home)
      const binaryPath = join(
        binaryDir,
        process.platform === "win32" ? "omnillm.exe" : "omnillm",
      )
      await runCommand("go", ["build", "-o", binaryPath, "main.go"])
      const port = await allocatePort()
      console.log(`Live matrix gateway: http://${HOST}:${port}`)
      gateway = await launchGateway(binaryPath, home, port, budgets.startupMs)
      const rowFailures = new Map<string, string>()
      for (const row of runnableRows) {
        try {
          await provisionRow(row, gateway.baseUrl, budgets.requestMs)
        } catch (error) {
          rowFailures.set(row.id, sanitizeFailure(error, secrets))
        }
      }
      const exactCacheRows = runnableRows.filter(
        (row) => !rowFailures.has(row.id),
      )
      if (needsExactResponseCache(mode, exactCacheRows)) {
        exactCacheConfigurationFailure = await setExactResponseCacheEnabled(
          gateway.baseUrl,
          budgets.requestMs,
          false,
        )
      }
      const suiteController = new AbortController()
      suiteAbortSignal = suiteController.signal
      const execution = executeRows(
        manifest,
        mode,
        gateway.baseUrl,
        budgets.requestMs,
        secrets,
        missingByRow,
        rowFailures,
      )
      const suiteTimer = setTimeout(
        () => suiteController.abort(),
        budgets.suiteMs,
      )
      try {
        results = await execution
      } finally {
        clearTimeout(suiteTimer)
        suiteAbortSignal = undefined
      }
    } else {
      results = await executeRows(
        manifest,
        mode,
        "http://127.0.0.1:0",
        budgets.requestMs,
        secrets,
        missingByRow,
      )
    }
  } catch (error) {
    const reason = sanitizeFailure(error, secrets)
    for (const row of manifest.rows) {
      for (const shape of SHAPES) {
        for (const scenario of plannedScenarios(mode)) {
          if (
            preclassifiedResult(
              row,
              shape,
              scenario,
              mode,
              missingByRow.get(row.id) ?? [],
            )
          )
            continue
          results.push(result(row, shape, scenario, mode, "fail", 0, reason))
        }
      }
    }
  } finally {
    exactCacheConfigurationFailure = undefined
    if (gateway) await stopGateway(gateway)
    if (tempRoot) rmSync(tempRoot, { recursive: true, force: true })
  }

  const report: MatrixReport = {
    version: 1,
    mode,
    startedAt,
    finishedAt: new Date().toISOString(),
    manifest: basename(sourceManifest),
    ...(gateway ? { gateway: { host: gateway.host, port: gateway.port } } : {}),
    results,
    summary: summary(results),
  }
  const reportPath = writeReport(report)
  printSummary(report, reportPath)
  return report.summary.fail > 0 ? 1 : 0
}

if (import.meta.main) {
  process.exitCode = await main()
}
