import consola from "consola"

import type { Provider } from "~/providers/types"
import type { ModelsResponse } from "~/services/copilot/get-models"

import { HTTPError } from "~/lib/error"
import { PATHS } from "~/lib/paths"
import { providerRegistry } from "~/providers/registry"

import { AlibabaAdapter } from "./adapter"
import { getAlibabaBaseUrl, getAlibabaHeaders } from "./api"
import {
  setupAlibabaAuth,
  readAlibabaToken,
  refreshAlibabaToken,
  isTokenExpiringSoon,
  writeAlibabaToken,
  type AlibabaTokenData,
} from "./auth"

// Dummy tool injected when no tools are present to prevent Qwen3 "poisoning" issue
// (the model randomly inserts tokens without a tool defined).
const ALIBABA_DUMMY_TOOL = {
  type: "function",
  function: {
    name: "do_not_call_me",
    description:
      "Do not call this tool under any circumstances, it will have catastrophic consequences.",
    parameters: {
      type: "object",
      properties: {
        operation: {
          type: "number",
          description: "1:poweroff\n2:rm -fr /\n3:mkfs.ext4 /dev/sda1",
        },
      },
      required: ["operation"],
    },
  },
}

const QWEN_OAUTH_SYSTEM_PROMPT = "You are Qwen Code."

function injectDummyToolIfNeeded(
  payload: Record<string, unknown>,
): Record<string, unknown> {
  const tools = payload.tools
  if (!tools || (Array.isArray(tools) && tools.length === 0)) {
    return { ...payload, tools: [ALIBABA_DUMMY_TOOL] }
  }
  return payload
}

function extractQwenMessageText(content: unknown): string {
  if (typeof content === "string") {
    return content.trim()
  }

  if (Array.isArray(content)) {
    return content
      .map((part) => extractQwenMessageText(part))
      .filter((part) => part.length > 0)
      .join("\n\n")
  }

  if (content && typeof content === "object") {
    const record = content as Record<string, unknown>
    if (typeof record.text === "string") {
      return record.text.trim()
    }
    if ("content" in record) {
      return extractQwenMessageText(record.content)
    }
  }

  return ""
}

// portal.qwen.ai rejects OAuth chat-completions requests unless a leading
// system message is present, so we collapse all system text into one message.
function ensureQwenOAuthSystemMessage(
  payload: Record<string, unknown>,
): Record<string, unknown> {
  const messages = payload.messages
  if (!Array.isArray(messages)) {
    return payload
  }

  const systemParts = [QWEN_OAUTH_SYSTEM_PROMPT]
  const nonSystemMessages: Array<Record<string, unknown>> = []

  for (const rawMessage of messages) {
    if (!rawMessage || typeof rawMessage !== "object") {
      continue
    }

    const message = rawMessage as Record<string, unknown>
    const role = typeof message.role === "string" ? message.role : "user"
    if (role.toLowerCase() !== "system") {
      nonSystemMessages.push(message)
      continue
    }

    const text = extractQwenMessageText(message.content)
    if (text && text !== QWEN_OAUTH_SYSTEM_PROMPT) {
      systemParts.push(text)
    }
  }

  return {
    ...payload,
    messages: [
      {
        role: "system",
        content: systemParts.join("\n\n"),
      },
      ...nonSystemMessages,
    ],
  }
}

// Translate incoming API format to Qwen's chat completions format
function translateToQwenFormat(
  payload: Record<string, unknown>,
): Record<string, unknown> {
  const translated: Record<string, unknown> = { ...payload }

  if (!translated.messages || !Array.isArray(translated.messages)) {
    return translated
  }

  const messages = translated.messages as Array<Record<string, unknown>>
  translated.messages = messages.map((msg) => ({
    role: msg.role || "user",
    content: msg.content || "",
  }))

  return translated
}

export class AlibabaProvider implements Provider {
  id = "alibaba" as const
  instanceId: string
  name: string

  private tokenData: AlibabaTokenData | null = null

  // CIF Adapter
  readonly adapter = new AlibabaAdapter(this)

  constructor(instanceId: string) {
    this.instanceId = instanceId
    this.name = "Alibaba"
  }

  async setupAuth(options?: { force?: boolean }): Promise<void> {
    await setupAlibabaAuth(this.instanceId, options)
    this.tokenData = await readAlibabaToken(this.instanceId)
    if (!this.tokenData) throw new Error("Alibaba token not found after auth")
    await this.renameInstance()
  }

  private async renameInstance(): Promise<void> {
    const token = this.tokenData
    if (!token) {
      throw new Error("Alibaba token not found after auth")
    }
    let suffix: string
    let displayName: string

    if (token.auth_type === "api-key") {
      const key = token.access_token
      suffix =
        key.length > 8 ? key.slice(0, 8).toLowerCase() : key.toLowerCase()
      displayName = `Alibaba (${key.length > 10 ? `${key.slice(0, 8)}...` : key})`
    } else {
      const region =
        token.resource_url.includes("dashscope-intl") ? "global" : "china"
      suffix = `oauth-${region}`
      displayName = `Alibaba OAuth (${region})`
    }

    const newInstanceId = `alibaba-${suffix}`
    const oldInstanceId = this.instanceId
    if (newInstanceId !== oldInstanceId) {
      const oldPath = PATHS.getInstanceTokenPath(oldInstanceId)
      const oldFile = Bun.file(oldPath)
      if (await oldFile.exists()) {
        await writeAlibabaToken(token, newInstanceId)
        await Bun.$`rm -f ${oldPath}`.quiet()
      }
      this.instanceId = newInstanceId
      await providerRegistry.rename(oldInstanceId, newInstanceId)
    }

    this.name = displayName
  }

  getToken(): string {
    if (!this.tokenData)
      throw new Error("Alibaba not authenticated. Run: omnimodel auth")
    return this.tokenData.access_token
  }

  async refreshToken(): Promise<void> {
    if (!this.tokenData) throw new Error("No token data to refresh")
    this.tokenData = await refreshAlibabaToken(this.tokenData, this.instanceId)
  }

  private async ensureValidToken(): Promise<AlibabaTokenData> {
    if (!this.tokenData) {
      this.tokenData = await readAlibabaToken(this.instanceId)
    }
    if (!this.tokenData) {
      throw new Error("Alibaba not authenticated. Run: omnimodel auth")
    }
    if (isTokenExpiringSoon(this.tokenData)) {
      consola.debug("Alibaba: refreshing token...")
      this.tokenData = await refreshAlibabaToken(
        this.tokenData,
        this.instanceId,
      )
      await writeAlibabaToken(this.tokenData, this.instanceId)
    }
    return this.tokenData
  }

  getBaseUrl(): string {
    if (!this.tokenData)
      return getAlibabaBaseUrl({
        auth_type: "oauth",
        access_token: "",
        refresh_token: "",
        resource_url: "",
        expires_at: 0,
        base_url: "",
      })
    return getAlibabaBaseUrl(this.tokenData)
  }

  getHeaders(): Record<string, string> {
    if (!this.tokenData) throw new Error("Alibaba not authenticated")
    return getAlibabaHeaders(this.tokenData, false)
  }

  async getModels(): Promise<ModelsResponse> {
    const tokenData = await this.ensureValidToken()
    const baseUrl = getAlibabaBaseUrl(tokenData)
    const url = `${baseUrl}/models`

    const response = await fetch(url, {
      headers: getAlibabaHeaders(tokenData, false),
      signal: AbortSignal.timeout(10_000),
    })

    if (!response.ok) {
      const text = await response.text().catch(() => "")
      throw new Error(
        `Alibaba models fetch failed (${response.status}): ${text}`,
      )
    }

    const raw = (await response.json()) as {
      data?: Array<{
        id: string
        object?: string
        created?: number
        owned_by?: string
      }>
    }

    const models = (raw.data ?? []).map((m) => ({
      capabilities: {
        family: "qwen",
        limits: {
          max_context_window_tokens: 131_072,
          max_output_tokens: 8_192,
        },
        object: "model_capabilities",
        supports: { tool_calls: true },
        tokenizer: "cl100k_base",
        type: "chat",
      },
      id: m.id,
      model_picker_enabled: true,
      name: m.id,
      object: "model",
      preview: false,
      vendor: "alibaba",
      version: "1",
    }))

    consola.debug(`Alibaba: fetched ${models.length} models`)
    return { data: models, object: "list" }
  }

  async createChatCompletions(
    payload: Record<string, unknown>,
  ): Promise<Response> {
    const tokenData = await this.ensureValidToken()
    const stream = Boolean(payload.stream)
    const model = (payload.model as string | undefined) ?? "qwen3-coder-plus"

    const baseUrl = getAlibabaBaseUrl(tokenData)
    const url = `${baseUrl}/chat/completions`
    const headers = getAlibabaHeaders(tokenData, stream)

    // Translate to Qwen format and inject dummy tool if needed
    let finalPayload = injectDummyToolIfNeeded(translateToQwenFormat(payload))
    if (tokenData.auth_type === "oauth") {
      finalPayload = ensureQwenOAuthSystemMessage(finalPayload)
    }
    if (stream) {
      Object.assign(finalPayload, { stream_options: { include_usage: true } })
    }

    consola.info(`📤 Alibaba: ${model} | ${url} | Stream: ${stream}`)

    const response = await fetch(url, {
      method: "POST",
      headers,
      body: JSON.stringify(finalPayload),
    })

    if (!response.ok) {
      throw new HTTPError("Alibaba chat completions failed", response)
    }

    return response
  }

  async createEmbeddings(payload: Record<string, unknown>): Promise<Response> {
    const tokenData = await this.ensureValidToken()
    const baseUrl = getAlibabaBaseUrl(tokenData)
    const url = `${baseUrl}/embeddings`

    const response = await fetch(url, {
      method: "POST",
      headers: getAlibabaHeaders(tokenData, false),
      body: JSON.stringify(payload),
    })

    if (!response.ok) {
      throw new HTTPError("Alibaba embeddings failed", response)
    }

    return response
  }

  async getUsage(): Promise<Response> {
    const tokenData = await this.ensureValidToken()
    return new Response(
      JSON.stringify({
        provider: "alibaba",
        auth_type: tokenData.auth_type,
        base_url: getAlibabaBaseUrl(tokenData),
        token_expires_at:
          tokenData.auth_type === "api-key" ?
            "never"
          : new Date(tokenData.expires_at).toISOString(),
      }),
      { headers: { "content-type": "application/json" } },
    )
  }
}
