import { createLogger } from "@/lib/logger"

const log = createLogger("api")

// ─── Shared types ─────────────────────────────────────────────────────────────

export interface Provider {
  id: string // Instance ID (e.g., "antigravity-1", "alibaba-2")
  type: string // Provider type (e.g., "antigravity", "alibaba")
  name: string
  isActive: boolean
  authStatus: "authenticated" | "unauthenticated"
  enabledModelCount?: number
  totalModelCount?: number
  priority?: number
  config?: {
    endpoint?: string
    apiVersion?: string
    deployments?: Array<string>
  }
}

export interface AuthFlow {
  providerId: string
  status: "pending" | "awaiting_user" | "complete" | "error"
  instructionURL?: string
  userCode?: string
  error?: string
}

export interface Status {
  activeProvider: { id: string; name: string } | null
  modelCount: number
  manualApprove: boolean
  rateLimitSeconds: number | null
  rateLimitWait: boolean
  authFlow: AuthFlow | null
}

export interface ServerInfo {
  version: string
  port: number
}

export type LogLevel =
  | "fatal"
  | "error"
  | "warn"
  | "info"
  | "debug"
  | "trace"

const LOG_LEVEL_BY_SEVERITY = [
  "fatal",
  "error",
  "warn",
  "info",
  "debug",
  "trace",
] as const

function normalizeLogLevel(value: unknown): LogLevel {
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase()
    if (
      (LOG_LEVEL_BY_SEVERITY as ReadonlyArray<string>).includes(normalized)
    ) {
      return normalized as LogLevel
    }

    if (normalized === "warning") {
      return "warn"
    }
  }

  if (
    typeof value === "number"
    && Number.isInteger(value)
    && value >= 0
    && value < LOG_LEVEL_BY_SEVERITY.length
  ) {
    return LOG_LEVEL_BY_SEVERITY[value] as LogLevel
  }

  return "info"
}

export interface Model {
  id: string
  name: string
  vendor?: string
  enabled: boolean
}

export interface ModelInfo {
  id: string
  display_name?: string
  name?: string
  owned_by?: string
  api_shape?: string
}

export interface QuotaSnapshot {
  entitlement: number
  remaining: number
  quota_remaining?: number
  percent_remaining: number
  unlimited: boolean
  overage_count?: number
  overage_permitted?: boolean
  quota_id?: string
}

export interface ChatMessage {
  role: "user" | "assistant" | "system"
  content: string
}

export interface ChatCompletionRequest {
  model: string
  messages: Array<ChatMessage>
  stream?: boolean
  max_tokens?: number
  temperature?: number
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: ChatMessage
    finish_reason: string | null
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface AnthropicContentBlock {
  type: "text" | "tool_use"
  text?: string
  id?: string
  name?: string
  input?: Record<string, unknown>
}

export interface AnthropicResponse {
  id: string
  type: "message"
  role: "assistant"
  content: Array<AnthropicContentBlock>
  model: string
  stop_reason: string
  usage?: {
    input_tokens: number
    output_tokens: number
  }
}

export interface ResponsesInputItem {
  type: "message"
  role: "user" | "assistant" | "system"
  content: string
}

export interface ResponsesRequest {
  model: string
  input: Array<ResponsesInputItem>
  max_output_tokens?: number
  stream?: boolean
  temperature?: number
}

export interface ResponsesResponse {
  id: string
  object: "realtime.response"
  model: string
  output: Array<{
    type: "message"
    id: string
    role: "assistant"
    content: Array<{
      type: "output_text"
      text: string
    }>
  }>
  usage?: {
    input_tokens: number
    output_tokens: number
  }
  created_at?: number
}

export type ChatApiResponse = ChatCompletionResponse | AnthropicResponse | ResponsesResponse

export interface UsageData {
  // GitHub Copilot fields
  access_type_sku?: string
  copilot_plan?: string
  quota_reset_date?: string
  chat_enabled?: boolean
  assigned_date?: string
  quota_snapshots?: Record<string, QuotaSnapshot>
  [key: string]: unknown
}

// ─── Base fetch ───────────────────────────────────────────────────────────────

// Auto-detect backend port at runtime
let detectedBackendPort: number | null = null
let detectionPromise: Promise<number> | null = null

async function detectBackendPort(): Promise<number> {
  if (detectedBackendPort) {
    return detectedBackendPort
  }

  if (detectionPromise) {
    return detectionPromise
  }

  detectionPromise = (async () => {
    const possiblePorts = [
      // Try compile-time port first if available
      typeof __SERVER_PORT__ !== "undefined" ?
        Number(__SERVER_PORT__)
      : undefined,
      Number(import.meta.env.VITE_SERVER_PORT), // Vite env variable
      4141, // default from start.ts
      5000, // common go backend port override
      5002, // common alternative
      3000, // common dev port
      8000, // another common port
    ].filter((port): port is number => Boolean(port && !Number.isNaN(port)))

    for (const port of possiblePorts) {
      try {
        log.trace(`trying backend port ${port}`)
        const response = await fetch(
          `${globalThis.location.protocol}//${globalThis.location.hostname}:${port}/api/admin/info`,
          {
            method: "GET",
            signal: AbortSignal.timeout(2000),
          },
        )
        if (response.ok) {
          detectedBackendPort = port
          log.info(`Auto-detected backend server on port ${port}`)
          return port
        }
      } catch {
        // Port not available, continue checking
      }
    }

    // If we're running on a different port than 5080, assume backend is on a different port too
    const currentPort = Number(globalThis.location.port)
    if (currentPort && currentPort !== 5080) {
      // Try the port that's 1000 less (common pattern: frontend 5173, backend 4173)
      const guessedPort = currentPort - 1000
      try {
        const response = await fetch(
          `${globalThis.location.protocol}//${globalThis.location.hostname}:${guessedPort}/api/admin/info`,
          {
            method: "GET",
            signal: AbortSignal.timeout(2000),
          },
        )
        if (response.ok) {
          detectedBackendPort = guessedPort
          log.info(`Auto-detected backend server on port ${guessedPort} (frontend port - 1000)`)
          return guessedPort
        }
      } catch {
        // Guess failed
      }
    }

    // Last resort: use compile-time port or default
    const fallback =
      typeof __SERVER_PORT__ !== "undefined" ? Number(__SERVER_PORT__) : 4141
    console.warn(
      `⚠️ Could not auto-detect backend port, falling back to ${fallback}`,
    )
    detectedBackendPort = fallback
    return fallback
  })()

  return detectionPromise
}

async function getBackendBase(): Promise<string> {
  // If we're in development and running on localhost with a port, auto-detect
  if (
    globalThis.location.hostname === "localhost"
    && globalThis.location.port
  ) {
    const port = await detectBackendPort()
    return `${globalThis.location.protocol}//${globalThis.location.hostname}:${port}`
  }

  // In production or when served from the same host, assume relative URLs work
  return ""
}

async function apiFetch<T>(path: string, opts: RequestInit = {}): Promise<T> {
  const backendBase = await getBackendBase()
  const fullUrl = `${backendBase}${path}`
  const method = opts.method || "GET"

  const t0 = performance.now()
  log.trace(`${method} ${path}`, { url: fullUrl })

  const res = await fetch(fullUrl, {
    headers: { "Content-Type": "application/json" },
    ...opts,
  })

  const ms = Math.round(performance.now() - t0)
  if (!res.ok) {
    const body = (await res
      .json()
      .catch(() => ({}) as Record<string, unknown>)) as Record<string, unknown>
    log.error(`${method} ${path} HTTP ${res.status}`, {
      durationMs: ms,
      error: body.error,
    })
    // eslint-disable-next-line @typescript-eslint/no-base-to-string
    throw new Error(String(body.error || `HTTP ${res.status}`))
  }

  log.debug(`${method} ${path} ${res.status} ${ms}ms`)
  return res.json() as Promise<T>
}

// ─── Providers ────────────────────────────────────────────────────────────────

export const listProviders = () =>
  apiFetch<Array<Provider>>("/api/admin/providers")

export const switchProvider = (providerId: string) =>
  apiFetch<{
    success?: boolean
    requiresAuth?: boolean
    provider?: { id: string; name: string }
  }>("/api/admin/providers/switch", {
    method: "POST",
    body: JSON.stringify({ providerId }),
  })

export const getProviderModels = (id: string) =>
  apiFetch<{ models: Array<Model> }>(`/api/admin/providers/${id}/models`)

export const activateProvider = (id: string) =>
  apiFetch<{ success?: boolean; provider?: { id: string; name: string } }>(
    `/api/admin/providers/${id}/activate`,
    { method: "POST" },
  )

export const deactivateProvider = (id: string) =>
  apiFetch<{ success?: boolean }>(`/api/admin/providers/${id}/deactivate`, {
    method: "POST",
  })

export const deleteProvider = (id: string) =>
  apiFetch<{ success?: boolean; message?: string }>(
    `/api/admin/providers/${id}`,
    {
      method: "DELETE",
    },
  )

export const toggleProviderModel = (
  id: string,
  modelId: string,
  enabled: boolean,
) =>
  apiFetch<{ success?: boolean; modelId: string; enabled: boolean }>(
    `/api/admin/providers/${id}/models/toggle`,
    { method: "POST", body: JSON.stringify({ modelId, enabled }) },
  )

export const getProviderPriorities = () =>
  apiFetch<{ priorities: Record<string, number> }>(
    "/api/admin/providers/priorities",
  )

export const setProviderPriorities = (priorities: Record<string, number>) =>
  apiFetch<{ success?: boolean }>("/api/admin/providers/priorities", {
    method: "POST",
    body: JSON.stringify({ priorities }),
  })

export const authProvider = (id: string, body: Record<string, string>) =>
  apiFetch<{ success?: boolean; requiresAuth?: boolean }>(
    `/api/admin/providers/${id}/auth`,
    { method: "POST", body: JSON.stringify(body) },
  )

export const addProviderInstance = (providerType: string) =>
  apiFetch<{ success?: boolean; provider?: Provider }>(
    `/api/admin/providers/add/${providerType}`,
    { method: "POST" },
  )

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export const updateProviderConfig = (id: string, config: Record<string, any>) =>
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  apiFetch<{ success?: boolean; config?: any }>(
    `/api/admin/providers/${id}/config`,
    { method: "PUT", body: JSON.stringify(config) },
  )

// ─── Status / Auth flow ───────────────────────────────────────────────────────

export const getStatus = () => apiFetch<Status>("/api/admin/status")

export const getAuthStatus = () =>
  apiFetch<AuthFlow | null>("/api/admin/auth-status")

export const cancelAuth = () =>
  apiFetch<{ success: boolean }>("/api/admin/auth/cancel", { method: "POST" })

// ─── Info ─────────────────────────────────────────────────────────────────────

export const getInfo = () => apiFetch<ServerInfo>("/api/admin/info")

// ─── Usage ────────────────────────────────────────────────────────────────────

export const getUsage = () => apiFetch<UsageData>("/usage")

export const getProviderUsage = (id: string) =>
  apiFetch<UsageData>(`/api/admin/providers/${id}/usage`)

// ─── Log level ────────────────────────────────────────────────────────────────

export const getLogLevel = async () => {
  const result = await apiFetch<{ level: unknown }>(
    "/api/admin/settings/log-level",
  )

  return { level: normalizeLogLevel(result.level) }
}

export const updateLogLevel = async (level: LogLevel) => {
  const result = await apiFetch<{ success: boolean; level: unknown }>(
    "/api/admin/settings/log-level",
    {
      method: "PUT",
      body: JSON.stringify({ level }),
    },
  )

  return {
    ...result,
    level: normalizeLogLevel(result.level),
  }
}

export async function subscribeToLogs(
  onLine: (line: string) => void,
): Promise<EventSource> {
  const backendBase = await getBackendBase()
  const url = `${backendBase}/api/admin/logs/stream`

  const es = new EventSource(url)
  es.addEventListener("message", (e: Event) => {
    const event = e as MessageEvent
    onLine(event.data as string)
  })
  es.addEventListener("error", () => {
    console.error(`❌ EventSource connection failed to ${url}`)
  })
  return es
}

export async function subscribeToLogsWebSocket(
  onLine: (line: string) => void,
): Promise<WebSocket> {
  const backendBase = await getBackendBase()
  const protocol = globalThis.location.protocol === "https:" ? "wss:" : "ws:"

  // Extract host and port from backendBase
  let host = globalThis.location.host
  if (backendBase && backendBase !== "") {
    const url = new URL(backendBase)
    host = url.host
  }

  const wsUrl = `${protocol}//${host}/api/admin/logs/websocket`
  const ws = new WebSocket(wsUrl)

  ws.addEventListener("message", (event: Event) => {
    const messageEvent = event as MessageEvent
    onLine(messageEvent.data as string)
  })
  ws.addEventListener("error", () => {
    console.error(`❌ WebSocket connection failed to ${wsUrl}`)
  })

  return ws
}

// ─── Models ────────────────────────────────────────────────────────────────

export const getModels = () =>
  apiFetch<{ object: string; data: Array<ModelInfo>; has_more: boolean }>("/models")

// ─── Chat Completions ──────────────────────────────────────────────────────

export const createChatCompletion = async (
  request: ChatCompletionRequest,
  apiShape: "openai" | "anthropic" | "responses" = "openai"
) => {
  let endpoint: string
  let requestBody: Record<string, unknown> = {}

  switch (apiShape) {
    case "anthropic": {
      endpoint = "/v1/messages"
      // Convert OpenAI-style messages to Anthropic format
      requestBody = {
        model: request.model,
        max_tokens: request.max_tokens || 1024,
        messages: request.messages,
        stream: request.stream || false,
        temperature: request.temperature
      } as Record<string, unknown>
      break
    }
    case "responses": {
      endpoint = "/v1/responses"
      // Convert OpenAI-style messages to Responses format
      requestBody = {
        model: request.model,
        input: request.messages.map(msg => ({
          type: "message",
          role: msg.role,
          content: msg.content
        })),
        max_output_tokens: request.max_tokens || 1024,
        stream: request.stream || false,
        temperature: request.temperature
      } as Record<string, unknown>
      break
    }
    default: {
      endpoint = "/v1/chat/completions"
      requestBody = request as Record<string, unknown>
      break
    }
  }

  if (request.stream) {
    // Return a ReadableStream for streaming responses
    const backendBase = await getBackendBase()
    const fullUrl = `${backendBase}${endpoint}`

    const response = await fetch(fullUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Accept": "text/event-stream"
      },
      body: JSON.stringify(requestBody),
    })

    if (!response.ok) {
      const body = (await response.json().catch(() => ({}))) as Record<string, unknown>
      const errorMsg = typeof body.error === "string" ? body.error : `HTTP ${response.status}`
      throw new Error(errorMsg)
    }

    return response.body
  } else {
    // Regular API call for non-streaming
    return apiFetch<ChatApiResponse>(endpoint, {
      method: "POST",
      body: JSON.stringify(requestBody),
    })
  }
}

// ─── Chat History ──────────────────────────────────────────────────────────

export interface ChatSessionSummary {
  session_id: string
  title: string
  model_id: string
  api_shape: string
  created_at: string
  updated_at: string
}

export interface ChatSessionDetail {
  session: ChatSessionSummary
  messages: Array<{ message_id: string; session_id: string; role: string; content: string; created_at: string }>
}

function normalizeSessionSummary(raw: Record<string, unknown>): ChatSessionSummary {
  const sessionId = raw.session_id ?? raw.id
  const title = raw.title
  const modelId = raw.model_id
  const apiShape = raw.api_shape ?? "openai"

  return {
    session_id: typeof sessionId === "string" ? sessionId : "",
    title: typeof title === "string" ? title : "",
    model_id: typeof modelId === "string" ? modelId : "",
    api_shape: typeof apiShape === "string" ? apiShape : "openai",
    created_at: typeof raw.created_at === "string" ? raw.created_at : "",
    updated_at: typeof raw.updated_at === "string" ? raw.updated_at : "",
  }
}

export const listChatSessions = async () => {
  const response = await apiFetch<
    Array<Record<string, unknown>> | { sessions?: Array<Record<string, unknown>> }
  >("/api/admin/chat/sessions")
  const sessions = Array.isArray(response) ? response : (response.sessions ?? [])
  return sessions.map(normalizeSessionSummary)
}

export const getChatSession = async (sessionId: string) => {
  const response = await apiFetch<Record<string, unknown> | ChatSessionDetail>(
    `/api/admin/chat/sessions/${sessionId}`,
  )

  // Node backend shape: { session: {...}, messages: [...] }
  if ("session" in response && response.session) {
    const typed = response as ChatSessionDetail
    return {
      session: normalizeSessionSummary(typed.session as unknown as Record<string, unknown>),
      messages: typed.messages.map((msg) => ({
        message_id: String(msg.message_id ?? ""),
        session_id: String(msg.session_id ?? sessionId),
        role: String(msg.role ?? ""),
        content: String(msg.content ?? ""),
        created_at: String(msg.created_at ?? ""),
      })),
    }
  }

  // Go backend shape: { id, title, ..., messages: [{ id, role, content, created_at }] }
  const raw = response as Record<string, unknown>
  const rawMessages = Array.isArray(raw.messages) ? raw.messages : []
  return {
    session: normalizeSessionSummary(raw),
    messages: rawMessages.map((msg) => {
      const rawMsg = msg as Record<string, unknown>
      return {
        message_id: String(rawMsg.message_id ?? rawMsg.id ?? ""),
        session_id: sessionId,
        role: String(rawMsg.role ?? ""),
        content: String(rawMsg.content ?? ""),
        created_at: String(rawMsg.created_at ?? ""),
      }
    }),
  }
}

export const createChatSession = (body: { session_id: string; title: string; model_id: string; api_shape: string }) =>
  apiFetch<{ ok?: boolean; success?: boolean; session_id?: string }>("/api/admin/chat/sessions", { method: "POST", body: JSON.stringify(body) })

export const addChatMessage = (sessionId: string, body: { message_id: string; role: string; content: string }) =>
  apiFetch<{ ok: boolean }>(`/api/admin/chat/sessions/${sessionId}/messages`, { method: "POST", body: JSON.stringify(body) })

export const deleteChatSession = (sessionId: string) =>
  apiFetch<{ ok: boolean }>(`/api/admin/chat/sessions/${sessionId}`, { method: "DELETE" })

export const deleteAllChatSessions = () =>
  apiFetch<{ ok: boolean }>("/api/admin/chat/sessions", { method: "DELETE" })

// ─── Virtual models ────────────────────────────────────────────────────────────

export type LbStrategy = "round-robin" | "random" | "priority" | "weighted"

export interface VirtualModelUpstream {
  id?: number
  virtual_model_id?: string
  provider_id?: string
  model_id: string
  weight: number
  priority: number
}

export interface VirtualModel {
  virtual_model_id: string
  name: string
  description: string
  api_shape: string
  lb_strategy: LbStrategy
  enabled: boolean
  created_at?: string
  updated_at?: string
  upstreams: Array<VirtualModelUpstream>
}

export interface VirtualModelPayload {
  virtual_model_id: string
  name: string
  description?: string
  api_shape?: string
  lb_strategy: LbStrategy
  enabled?: boolean
  upstreams: Array<{ provider_id?: string; model_id: string; weight?: number; priority?: number }>
}

export const listVirtualModels = () =>
  apiFetch<{ data: Array<VirtualModel> }>("/api/admin/vmodels").then((r) => r.data ?? [])

export const getVirtualModel = (id: string) =>
  apiFetch<VirtualModel>(`/api/admin/vmodels/${encodeURIComponent(id)}`)

export const createVirtualModel = (payload: VirtualModelPayload) =>
  apiFetch<VirtualModel>("/api/admin/vmodels", {
    method: "POST",
    body: JSON.stringify(payload),
  })

export const updateVirtualModel = (id: string, payload: VirtualModelPayload) =>
  apiFetch<VirtualModel>(`/api/admin/vmodels/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: JSON.stringify(payload),
  })

export const deleteVirtualModel = (id: string) =>
  apiFetch<{ deleted: string }>(`/api/admin/vmodels/${encodeURIComponent(id)}`, {
    method: "DELETE",
  })
