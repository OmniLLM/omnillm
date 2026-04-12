import consola from "consola"

import { state } from "~/lib/state"

import {
  ALIBABA_OAUTH_CLIENT_ID,
  ALIBABA_OAUTH_DEVICE_CODE_ENDPOINT,
  ALIBABA_OAUTH_GRANT_TYPE,
  ALIBABA_OAUTH_SCOPE,
  ALIBABA_OAUTH_TOKEN_ENDPOINT,
  ALIBABA_REFRESH_SKEW_MS,
  ALIBABA_BASE_URL_CHINA,
  ALIBABA_BASE_URL_GLOBAL,
} from "./constants"

export type AlibabaAuthType = "oauth" | "api-key"

export interface AlibabaTokenData {
  auth_type: AlibabaAuthType
  access_token: string
  // OAuth fields
  refresh_token: string
  resource_url: string // base URL override from OAuth token response
  expires_at: number // unix timestamp (ms); 0 for api-key (never expires)
  // API key fields
  base_url: string // explicit base URL for api-key auth
}

export const readAlibabaToken = async (
  instanceId: string,
): Promise<AlibabaTokenData | null> => {
  const { readAlibabaToken: dbReadToken } = await import("~/lib/token-db")
  return await dbReadToken(instanceId)
}

export const writeAlibabaToken = async (
  data: AlibabaTokenData,
  instanceId: string,
): Promise<void> => {
  const { writeAlibabaToken: dbWriteToken } = await import("~/lib/token-db")
  await dbWriteToken(data, instanceId)
}

// ─── OAuth device flow ───────────────────────────────────────────────────────

interface DeviceFlowResponse {
  device_code: string
  user_code: string
  verification_uri: string
  verification_uri_complete?: string
  expires_in: number
  interval?: number
}

interface TokenResponse {
  access_token: string
  refresh_token?: string
  token_type: string
  resource_url?: string
  expires_in: number
}

async function initiateDeviceFlow(): Promise<DeviceFlowResponse> {
  const body = new URLSearchParams({
    client_id: ALIBABA_OAUTH_CLIENT_ID,
    scope: ALIBABA_OAUTH_SCOPE,
  })

  const headers: Record<string, string> = {
    "Content-Type": "application/x-www-form-urlencoded",
    Accept: "application/json",
  }
  const requestId = globalThis.crypto.randomUUID()
  headers["x-request-id"] = requestId

  const response = await fetch(ALIBABA_OAUTH_DEVICE_CODE_ENDPOINT, {
    method: "POST",
    headers,
    body: body.toString(),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`Device authorization failed (${response.status}): ${text}`)
  }

  return response.json() as Promise<DeviceFlowResponse>
}

interface PollForTokenOptions {
  deviceCode: string
  intervalSeconds: number | undefined
  expiresIn: number
}

async function pollForToken(
  opts: PollForTokenOptions,
): Promise<AlibabaTokenData> {
  const { deviceCode, intervalSeconds, expiresIn } = opts
  const deadline = Date.now() + expiresIn * 1000
  const resolvedIntervalMs =
    typeof intervalSeconds === "number" && intervalSeconds > 0 ?
      Math.max(1000, intervalSeconds * 1000)
    : 5000
  let pollInterval = resolvedIntervalMs

  while (Date.now() < deadline) {
    await Bun.sleep(pollInterval)

    const body = new URLSearchParams({
      grant_type: ALIBABA_OAUTH_GRANT_TYPE,
      client_id: ALIBABA_OAUTH_CLIENT_ID,
      device_code: deviceCode,
    })

    const response = await fetch(ALIBABA_OAUTH_TOKEN_ENDPOINT, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        Accept: "application/json",
      },
      body: body.toString(),
    })

    const responseBody = (await response.json()) as Record<string, unknown>

    if (response.ok) {
      const tokenResp = responseBody as unknown as TokenResponse
      return {
        auth_type: "oauth",
        access_token: tokenResp.access_token,
        refresh_token: tokenResp.refresh_token ?? "",
        resource_url: tokenResp.resource_url ?? "",
        expires_at: Date.now() + tokenResp.expires_in * 1000 - 5 * 60 * 1000,
        base_url: "",
      }
    }

    if (response.status === 400) {
      const errorType = responseBody["error"] as string | undefined
      switch (errorType) {
        case "authorization_pending": {
          consola.debug("Alibaba: waiting for authorization...")
          continue
        }
        case "slow_down": {
          pollInterval = Math.min(pollInterval + 5000, 10_000)
          consola.debug(
            `Alibaba: server requested slow down, polling every ${pollInterval}ms`,
          )
          continue
        }
        case "expired_token": {
          throw new Error("Device code expired. Please restart authentication.")
        }
        case "access_denied": {
          throw new Error(
            "Authorization denied. Please restart authentication.",
          )
        }
        default: {
          const desc = responseBody["error_description"]
          throw new Error(
            `Token poll failed: ${errorType} - ${typeof desc === "string" ? desc : ""}`,
          )
        }
      }
    }

    throw new Error(
      `Token poll failed (${response.status}): ${JSON.stringify(responseBody)}`,
    )
  }

  throw new Error("Authentication timed out. Please restart authentication.")
}

async function setupOAuth(instanceId: string): Promise<void> {
  consola.debug("Alibaba: initiating device flow...")
  const deviceFlow = await initiateDeviceFlow()

  if (state.authFlow) {
    state.authFlow.status = "awaiting_user"
    state.authFlow.instructionURL =
      deviceFlow.verification_uri_complete ?? deviceFlow.verification_uri
    state.authFlow.userCode = deviceFlow.user_code
  }

  consola.info("")
  consola.info(
    "Alibaba authentication: visit the following URL and enter the code below",
  )
  consola.info(
    `URL: ${deviceFlow.verification_uri_complete ?? deviceFlow.verification_uri}`,
  )
  consola.info(`Code: ${deviceFlow.user_code}`)
  consola.info("")

  try {
    const url =
      deviceFlow.verification_uri_complete ?? deviceFlow.verification_uri
    if (process.platform === "win32") {
      await Bun.$`powershell -Command "Start-Process '${url}'"`.quiet()
    } else if (process.platform === "darwin") {
      await Bun.$`open ${url}`.quiet()
    } else {
      await Bun.$`xdg-open ${url}`.quiet()
    }
  } catch {
    // ignore browser open errors
  }

  consola.info("Waiting for authorization...")
  const tokenData = await pollForToken({
    deviceCode: deviceFlow.device_code,
    intervalSeconds: deviceFlow.interval,
    expiresIn: deviceFlow.expires_in,
  })

  await writeAlibabaToken(tokenData, instanceId)
  consola.success(
    `Alibaba OAuth authentication successful for instance ${instanceId}`,
  )
}

// ─── API key auth ─────────────────────────────────────────────────────────────

async function setupApiKey(instanceId: string): Promise<void> {
  const region = await consola.prompt("Select region", {
    type: "select",
    options: [
      { label: "China  (dashscope.aliyuncs.com)", value: "china" },
      { label: "Global (dashscope-intl.aliyuncs.com)", value: "global" },
    ],
  })

  const baseUrl =
    region === "global" ? ALIBABA_BASE_URL_GLOBAL : ALIBABA_BASE_URL_CHINA

  const apiKey = await consola.prompt("Enter your DashScope API key", {
    type: "text",
  })

  if (!apiKey || typeof apiKey !== "string" || !apiKey.trim()) {
    throw new Error("API key cannot be empty")
  }

  const tokenData: AlibabaTokenData = {
    auth_type: "api-key",
    access_token: apiKey.trim(),
    refresh_token: "",
    resource_url: "",
    expires_at: 0,
    base_url: baseUrl,
  }

  await writeAlibabaToken(tokenData, instanceId)
  consola.success(
    `Alibaba API key saved (${region} region) for instance ${instanceId}`,
  )
}

// ─── Public API ──────────────────────────────────────────────────────────────

export async function setupAlibabaAuth(
  instanceId: string,
  options?: {
    force?: boolean
  },
): Promise<void> {
  const existing = await readAlibabaToken(instanceId)
  if (existing && !options?.force) {
    consola.info(`Alibaba: already logged in (instance: ${instanceId})`)
    return
  }

  const method = await consola.prompt("Select authentication method", {
    type: "select",
    options: [
      { label: "OAuth (login with Qwen account)", value: "oauth" },
      { label: "API key (DashScope API key)", value: "api-key" },
    ],
  })

  await (method === "api-key" ?
    setupApiKey(instanceId)
  : setupOAuth(instanceId))
}

export async function refreshAlibabaToken(
  data: AlibabaTokenData,
  instanceId: string,
): Promise<AlibabaTokenData> {
  if (data.auth_type === "api-key") {
    // API keys don't expire
    return data
  }

  if (!data.refresh_token) {
    throw new Error("No refresh token available. Please re-authenticate.")
  }

  const body = new URLSearchParams({
    grant_type: "refresh_token",
    refresh_token: data.refresh_token,
    client_id: ALIBABA_OAUTH_CLIENT_ID,
  })

  const response = await fetch(ALIBABA_OAUTH_TOKEN_ENDPOINT, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Accept: "application/json",
    },
    body: body.toString(),
  })

  if (!response.ok) {
    const text = await response.text()
    throw new Error(`Token refresh failed (${response.status}): ${text}`)
  }

  const refreshed = (await response.json()) as TokenResponse

  const updated: AlibabaTokenData = {
    auth_type: "oauth",
    access_token: refreshed.access_token,
    refresh_token: refreshed.refresh_token ?? data.refresh_token,
    resource_url: refreshed.resource_url ?? data.resource_url,
    expires_at: Date.now() + refreshed.expires_in * 1000 - 5 * 60 * 1000,
    base_url: "",
  }

  await writeAlibabaToken(updated, instanceId)
  return updated
}

export function isTokenExpiringSoon(data: AlibabaTokenData): boolean {
  if (data.auth_type === "api-key") return false
  return data.expires_at - Date.now() < ALIBABA_REFRESH_SKEW_MS
}
