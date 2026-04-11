import type { Context } from "hono"

import consola from "consola"

import type { Provider } from "~/providers/types"
import type { ProviderID } from "~/providers/types"
import type { ModelsResponse } from "~/services/copilot/get-models"

import { saveConfig } from "~/lib/config"
import {
  ModelStateStore,
  ProviderConfigStore,
  ModelConfigStore,
  ProviderInstanceStore,
} from "~/lib/database"
import { PATHS } from "~/lib/paths"
import {
  getAzureOpenAIConfig,
  saveAzureOpenAIConfig,
  saveAlibabaConfig,
} from "~/lib/provider-config"
import { state } from "~/lib/state"
import { readAlibabaToken, writeAlibabaToken } from "~/providers/alibaba/auth"
import { readAntigravityToken } from "~/providers/antigravity/auth"
import {
  readAzureOpenAIToken,
  writeAzureOpenAIToken,
} from "~/providers/azure-openai/auth"
import { providerRegistry } from "~/providers/registry"

// ─── Auto-discovery helper ───────────────────────────────────────────────────

let discoveryPromise: Promise<void> | null = null

/**
 * Ensures provider auto-discovery has run exactly once per server session
 */
async function ensureProvidersDiscovered(): Promise<void> {
  if (!discoveryPromise) {
    discoveryPromise = (async () => {
      const { discoverProviderInstances } = await import("~/auth")
      await discoverProviderInstances()
    })()
  }
  return discoveryPromise
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function activateProvider(provider: Provider, models: ModelsResponse) {
  providerRegistry.setActive(provider.instanceId)
  state.currentProvider = provider
  state.selectedProviderID = provider.instanceId
  state.models = models
  state.activeProviderIDs.add(provider.instanceId)
  state.providerModels.set(provider.instanceId, models)
}

async function isGitHubCopilotAuthenticated(): Promise<boolean> {
  // Check if any GitHub Copilot instances are authenticated
  const { providerRegistry } = await import("~/providers/registry")
  const githubInstances = providerRegistry.getInstancesOfType("github-copilot")

  for (const instance of githubInstances) {
    const { readGithubToken } = await import("~/providers/github-copilot/auth")
    const token = await readGithubToken(instance.instanceId)
    if (token && token.trim().length > 0) {
      return true
    }
  }

  // Fall back to legacy token for backwards compatibility
  const legacyFile = Bun.file(PATHS.GITHUB_TOKEN_PATH)
  if (await legacyFile.exists()) {
    const text = await legacyFile.text()
    return Boolean(text.trim())
  }

  return false
}

async function isAlibabaAuthenticated(instanceId: string): Promise<boolean> {
  const token = await readAlibabaToken(instanceId)
  return token !== null && Boolean(token.access_token)
}

async function isAntigravityAuthenticated(
  instanceId: string,
): Promise<boolean> {
  const token = await readAntigravityToken(instanceId)
  return token !== null && Boolean(token.access_token)
}

async function isAzureOpenAIAuthenticated(
  instanceId: string,
): Promise<boolean> {
  const token = await readAzureOpenAIToken(instanceId)
  return token !== null && Boolean(token.api_key)
}

async function getAuthStatus(
  provider: Provider,
): Promise<"authenticated" | "unauthenticated"> {
  switch (provider.id) {
    case "github-copilot": {
      // Check specific instance authentication
      const { readGithubToken } = await import(
        "~/providers/github-copilot/auth"
      )
      const token = await readGithubToken(provider.instanceId)
      return token && token.trim().length > 0 ?
          "authenticated"
        : "unauthenticated"
    }
    case "alibaba": {
      return (await isAlibabaAuthenticated(provider.instanceId)) ?
          "authenticated"
        : "unauthenticated"
    }
    case "antigravity": {
      return (await isAntigravityAuthenticated(provider.instanceId)) ?
          "authenticated"
        : "unauthenticated"
    }
    case "azure-openai": {
      return (await isAzureOpenAIAuthenticated(provider.instanceId)) ?
          "authenticated"
        : "unauthenticated"
    }
    default: {
      return "unauthenticated"
    }
  }
}

// ─── GET /api/admin/providers ─────────────────────────────────────────────────

export async function handleListProviders(c: Context) {
  // Ensure all provider instances are discovered and registered
  await ensureProvidersDiscovered()

  const providerMap = providerRegistry.getProviderMap()

  const providers = await Promise.all(
    Array.from(providerMap.values()).map(async (provider) => {
      // Get disabled models from database instead of state
      const disabled = ModelStateStore.getDisabledModels(provider.instanceId)

      // Get fresh model count for each provider instance
      let totalModelCount = 0
      let enabledModelCount = 0

      try {
        if ((await getAuthStatus(provider)) === "authenticated") {
          const models = await provider.getModels()
          totalModelCount = models.data.length
          enabledModelCount = totalModelCount - disabled.size
        }
      } catch (err) {
        consola.warn(`Failed to get models for ${provider.instanceId}:`, err)
        // Fall back to cached data
        const cached = state.providerModels.get(provider.instanceId)
        totalModelCount = cached?.data.length ?? 0
        enabledModelCount = totalModelCount - disabled.size
      }

      const authStatus = await getAuthStatus(provider)
      const azureConfig =
        provider.id === "azure-openai" ?
          await readAzureOpenAIToken(provider.instanceId)
        : null

      // Get provider configuration from database
      let config: any = undefined
      if (provider.id === "azure-openai") {
        const azureProviderConfig = getAzureOpenAIConfig(provider.instanceId)
        if (azureProviderConfig) {
          config = azureProviderConfig
        } else if (azureConfig) {
          // Fallback to token data for backwards compatibility
          config = {
            endpoint: azureConfig.endpoint,
            apiVersion: azureConfig.api_version,
            deployments: azureConfig.deployments,
          }
        }
      }

      return {
        id: provider.instanceId,
        type: provider.id,
        name: provider.name,
        isActive: providerRegistry.isActiveProvider(provider.instanceId),
        authStatus,
        enabledModelCount,
        totalModelCount,
        config,
      }
    }),
  )

  return c.json(providers)
}

// ─── GET /api/admin/status ────────────────────────────────────────────────────

export function handleGetStatus(c: Context) {
  const activeProvider =
    state.currentProvider ?
      { id: state.currentProvider.instanceId, name: state.currentProvider.name }
    : null

  return c.json({
    activeProvider,
    modelCount: state.models?.data.length ?? 0,
    manualApprove: state.manualApprove,
    rateLimitSeconds: state.rateLimitSeconds ?? null,
    rateLimitWait: state.rateLimitWait,
    authFlow: state.authFlow || null,
  })
}

// ─── GET /api/admin/auth-status ──────────────────────────────────────────────

export function handleGetAuthStatus(c: Context) {
  return c.json(state.authFlow ?? null)
}

// ─── GET /api/admin/providers/:id/models ─────────────────────────────────────

export async function handleListProviderModels(c: Context) {
  await ensureProvidersDiscovered()
  const instanceId = c.req.param("id")

  if (!providerRegistry.isRegistered(instanceId)) {
    return c.json({ error: `Unknown provider: ${instanceId}` }, 400)
  }

  try {
    const provider = providerRegistry.getProvider(instanceId)
    const models = await provider.getModels()
    const disabled = ModelStateStore.getDisabledModels(instanceId)
    const modelsWithEnabled = models.data.map((m) => ({
      ...m,
      enabled: !disabled.has(m.id),
    }))
    return c.json({ models: modelsWithEnabled })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return c.json({ error: message }, 500)
  }
}

// ─── GET /api/admin/providers/:id/usage ──────────────────────────────────────

export async function handleGetProviderUsage(c: Context) {
  const id = c.req.param("id") as ProviderID

  if (!providerRegistry.isRegistered(id)) {
    return c.json({ error: `Unknown provider: ${id}` }, 400)
  }

  try {
    const provider = providerRegistry.getProvider(id)
    const response = await provider.getUsage()
    return response
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return c.json({ error: message }, 500)
  }
}

// ─── GET /api/admin/info ──────────────────────────────────────────────────────

export function handleGetInfo(c: Context) {
  return c.json({
    version: "0.0.1",
    port: state.port,
  })
}

// ─── POST /api/admin/providers/switch ─────────────────────────────────────────

export async function handleSwitchProvider(c: Context) {
  const body = await c.req.json<{ providerId: string }>()
  const providerId = body.providerId as ProviderID

  if (!providerRegistry.isRegistered(providerId)) {
    return c.json({ error: `Unknown provider: ${providerId}` }, 400)
  }

  if (
    state.authFlow?.status === "pending"
    || state.authFlow?.status === "awaiting_user"
  ) {
    return c.json({ error: "An auth flow is already in progress" }, 409)
  }

  const provider = providerRegistry.getProvider(providerId)

  // Check if already authenticated — if so, switch immediately
  const alreadyAuthed = await getAuthStatus(provider)

  if (alreadyAuthed === "authenticated") {
    try {
      await provider.setupAuth()
      const models = await provider.getModels()
      activateProvider(provider, models)
      consola.info(`Provider switched to: ${provider.name}`)
      return c.json({
        success: true,
        provider: { id: provider.id, name: provider.name },
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      return c.json({ error: `Failed to switch provider: ${message}` }, 500)
    }
  }

  // Not authenticated — kick off background auth flow
  state.authFlow = { providerId, status: "pending" }

  void (async () => {
    try {
      await provider.setupAuth()
      const models = await provider.getModels()
      activateProvider(provider, models)
      state.authFlow = { providerId, status: "complete" }
      consola.info(`Provider switched to: ${provider.name} (after auth)`)
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      consola.error(`Auth flow failed for ${providerId}:`, message)
      state.authFlow = { providerId, status: "error", error: message }
    }
  })()

  return c.json({ requiresAuth: true, providerId })
}

// ─── POST /api/admin/providers/:id/auth/alibaba ──────────────────────────────

async function handleAlibabaAuth(c: Context, id: ProviderID) {
  const body = await c.req.json<Record<string, string>>()
  const method = body.method

  if (method !== "api-key") {
    // oauth-based: trigger background auth
    state.authFlow = { providerId: id, status: "pending" }
    const provider = providerRegistry.getProvider(id)

    void (async () => {
      try {
        await provider.setupAuth({ force: true })
        const models = await provider.getModels()
        activateProvider(provider, models)
        state.authFlow = { providerId: id, status: "complete" }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err)
        state.authFlow = { providerId: id, status: "error", error: message }
      }
    })()

    return c.json({ requiresAuth: true, providerId: id })
  }

  const { apiKey, region } = body
  if (!apiKey?.trim()) return c.json({ error: "apiKey is required" }, 400)

  const { ALIBABA_BASE_URL_CHINA, ALIBABA_BASE_URL_GLOBAL } = await import(
    "~/providers/alibaba/constants"
  )
  const baseUrl =
    region === "global" ? ALIBABA_BASE_URL_GLOBAL : ALIBABA_BASE_URL_CHINA

  await writeAlibabaToken(
    {
      auth_type: "api-key",
      access_token: apiKey.trim(),
      refresh_token: "",
      resource_url: "",
      expires_at: 0,
      base_url: baseUrl,
    },
    id,
  )

  try {
    const provider = providerRegistry.getProvider(id)
    await provider.setupAuth()
    const models = await provider.getModels()
    activateProvider(provider, models)
    return c.json({ success: true })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return c.json({ error: message }, 500)
  }
}

// ─── POST /api/admin/providers/:id/auth/azure-openai ──────────────────────────

async function handleAzureOpenAIAuth(c: Context, id: ProviderID) {
  const body = await c.req.json<Record<string, string>>()
  const { apiKey } = body

  if (!apiKey?.trim()) {
    return c.json({ error: "apiKey is required" }, 400)
  }

  // Check if configuration exists
  const existingConfig = getAzureOpenAIConfig(id)
  const existingToken = await readAzureOpenAIToken(id)

  if (!existingConfig && !existingToken?.endpoint) {
    return c.json(
      {
        error:
          "No configuration found. Please configure the provider first using the 'Configure' button.",
      },
      400,
    )
  }

  // Use configuration from database or fallback to token data
  let endpoint, api_version, resource_name, deployments

  if (existingConfig) {
    ;({ endpoint, api_version, resource_name, deployments } = existingConfig)
  } else if (existingToken) {
    endpoint = existingToken.endpoint
    api_version = existingToken.api_version
    resource_name = existingToken.resource_name
    deployments = existingToken.deployments
  } else {
    return c.json({ error: "Configuration is required" }, 400)
  }

  await writeAzureOpenAIToken(
    {
      auth_type: "api-key",
      api_key: apiKey.trim(),
      endpoint,
      api_version,
      resource_name,
      deployments,
    },
    id,
  )

  try {
    const provider = providerRegistry.getProvider(id)
    await provider.setupAuth()
    const models = await provider.getModels()
    activateProvider(provider, models)
    return c.json({ success: true })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return c.json({ error: message }, 500)
  }
}

// ─── POST /api/admin/providers/:id/auth/github ───────────────────────────────

async function handleGitHubAuth(c: Context, id: ProviderID) {
  const body = await c.req.json<Record<string, string>>()
  const method = body.method

  if (method !== "token") {
    // oauth-based: trigger background auth
    state.authFlow = { providerId: id, status: "pending" }
    const provider = providerRegistry.getProvider(id)

    void (async () => {
      try {
        await provider.setupAuth({ force: true })
        const models = await provider.getModels()
        activateProvider(provider, models)
        state.authFlow = { providerId: id, status: "complete" }
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err)
        state.authFlow = { providerId: id, status: "error", error: message }
      }
    })()

    return c.json({ requiresAuth: true, providerId: id })
  }

  const { token } = body
  if (!token?.trim()) return c.json({ error: "token is required" }, 400)

  // Write token to instance-specific location
  const { writeGithubToken } = await import("~/providers/github-copilot/auth")
  try {
    const provider = providerRegistry.getProvider(id)
    // Get user info first
    const { getGitHubUser } = await import("~/services/github/get-user")
    const user = await getGitHubUser(token.trim())

    // Write token with metadata to instance-specific file
    await writeGithubToken(token.trim(), user.login, id)

    // Refresh the provider's token setup
    await provider.refreshToken()
    const models = await provider.getModels()
    activateProvider(provider, models)
    return c.json({ success: true })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return c.json({ error: message }, 500)
  }
}

// ─── POST /api/admin/providers/:id/auth ──────────────────────────────────────

export async function handleProviderAuth(c: Context) {
  await ensureProvidersDiscovered()
  const id = c.req.param("id") as ProviderID

  if (!providerRegistry.isRegistered(id)) {
    return c.json({ error: `Unknown provider: ${id}` }, 400)
  }

  if (
    state.authFlow?.status === "pending"
    || state.authFlow?.status === "awaiting_user"
  ) {
    return c.json({ error: "An auth flow is already in progress" }, 409)
  }

  const provider = providerRegistry.getProvider(id)

  if (provider.id === "alibaba") {
    return handleAlibabaAuth(c, id)
  }

  if (provider.id === "github-copilot") {
    return handleGitHubAuth(c, id)
  }

  if (provider.id === "azure-openai") {
    return handleAzureOpenAIAuth(c, id)
  }

  // Antigravity: oauth only
  const body = await c.req.json<Record<string, string>>()
  const { clientId, clientSecret } = body

  if (!clientId?.trim() || !clientSecret?.trim()) {
    return c.json(
      { error: "clientId and clientSecret are required for Antigravity OAuth" },
      400,
    )
  }

  state.authFlow = { providerId: id, status: "pending" }

  void (async () => {
    try {
      await provider.setupAuth({
        force: true,
        clientId: clientId.trim(),
        clientSecret: clientSecret.trim(),
      })
      const models = await provider.getModels()
      activateProvider(provider, models)
      state.authFlow = { providerId: id, status: "complete" }
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err)
      state.authFlow = { providerId: id, status: "error", error: message }
    }
  })()

  return c.json({ requiresAuth: true, providerId: id })
}

// ─── POST /api/admin/providers/:id/activate ───────────────────────────────────

export async function handleActivateProvider(c: Context) {
  // Ensure all provider instances are discovered before checking if registered
  await ensureProvidersDiscovered()

  const instanceId = c.req.param("id")

  if (!providerRegistry.isRegistered(instanceId)) {
    return c.json({ error: `Unknown provider: ${instanceId}` }, 400)
  }

  const provider = providerRegistry.getProvider(instanceId)

  try {
    // Check if provider is authenticated - if not, just activate without auth
    let models: ModelsResponse | undefined
    const authStatus = await getAuthStatus(provider)

    if (authStatus === "authenticated") {
      // Only try to get models if already authenticated
      try {
        models = await provider.getModels()
        state.providerModels.set(instanceId, models)
      } catch (err) {
        // Don't fail activation if model fetching fails
        consola.warn(
          `Failed to fetch models for ${instanceId}, but activating anyway:`,
          err,
        )
      }
    }

    providerRegistry.addActive(instanceId)
    state.activeProviderIDs.add(instanceId)

    // Update primary provider if this is the first active one
    if (!state.currentProvider || !state.selectedProviderID) {
      state.currentProvider = provider
      state.selectedProviderID = instanceId
      if (models) {
        state.models = models
      }
    }

    consola.info(`Provider activated: ${provider.name} (auth: ${authStatus})`)
    return c.json({
      success: true,
      provider: {
        id: provider.instanceId,
        name: provider.name,
        authStatus,
      },
    })
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err)
    return c.json({ error: message }, 500)
  }
}

// ─── POST /api/admin/providers/:id/deactivate ─────────────────────────────────

export function handleDeactivateProvider(c: Context) {
  const instanceId = c.req.param("id")

  if (!providerRegistry.isRegistered(instanceId)) {
    return c.json({ error: `Unknown provider: ${instanceId}` }, 400)
  }

  providerRegistry.removeActive(instanceId)
  state.activeProviderIDs.delete(instanceId)

  // Update primary provider to whatever is still active
  const remaining = providerRegistry.getActiveProviders()
  if (remaining.length > 0) {
    const next = remaining[0]
    state.currentProvider = next
    state.selectedProviderID = next.instanceId
    state.models = state.providerModels.get(next.instanceId)
  } else {
    state.currentProvider = undefined
    state.selectedProviderID = undefined
    state.models = undefined
  }

  consola.info(`Provider deactivated: ${instanceId}`)
  return c.json({ success: true })
}

// ─── POST /api/admin/providers/:id/models/toggle ──────────────────────────────

export function handleToggleProviderModel(c: Context) {
  const instanceId = c.req.param("id")

  if (!providerRegistry.isRegistered(instanceId)) {
    return c.json({ error: `Unknown provider: ${instanceId}` }, 400)
  }

  return c.req.json<{ modelId: string; enabled: boolean }>().then((body) => {
    const { modelId, enabled } = body
    if (!modelId) return c.json({ error: "modelId is required" }, 400)

    // Store in database instead of memory
    ModelStateStore.set(instanceId, modelId, enabled)

    // Also update in-memory state for backwards compatibility
    if (!state.disabledModels.has(instanceId)) {
      state.disabledModels.set(instanceId, new Set())
    }
    const disabled = state.disabledModels.get(instanceId)!
    if (enabled) {
      disabled.delete(modelId)
    } else {
      disabled.add(modelId)
    }

    return c.json({ success: true, modelId, enabled })
  })
}

// ─── GET /api/admin/providers/priorities ──────────────────────────────────────

export function handleGetProviderPriorities(c: Context) {
  const priorities: Record<string, number> = {}
  for (const [id, priority] of state.providerPriorities.entries()) {
    priorities[id] = priority
  }
  return c.json({ priorities })
}

// ─── POST /api/admin/providers/priorities ─────────────────────────────────────

export function handleSetProviderPriorities(c: Context) {
  return c.req
    .json<{ priorities: Record<string, number> }>()
    .then(async (body) => {
      const { priorities } = body
      if (!priorities || typeof priorities !== "object") {
        return c.json({ error: "priorities object is required" }, 400)
      }

      state.providerPriorities.clear()
      for (const [id, priority] of Object.entries(priorities)) {
        if (typeof priority === "number") {
          state.providerPriorities.set(id as ProviderID, priority)
        }
      }

      consola.debug(
        "Provider priorities updated:",
        Object.fromEntries(state.providerPriorities),
      )
      await saveConfig()
      return c.json({ success: true })
    })
}

// ─── POST /api/admin/providers/:type/add-instance ────────────────────────────

export async function handleAddProviderInstance(c: Context) {
  const providerType = c.req.param("type") as ProviderID

  // Validate provider type
  if (
    !["alibaba", "antigravity", "azure-openai", "github-copilot"].includes(
      providerType,
    )
  ) {
    return c.json({ error: `Unsupported provider type: ${providerType}` }, 400)
  }

  // Generate next instance ID
  const instanceId = providerRegistry.nextInstanceId(providerType)

  try {
    // Import and create the appropriate provider instance
    let provider: Provider

    switch (providerType) {
      case "github-copilot": {
        const { GitHubCopilotProvider } = await import(
          "~/providers/github-copilot"
        )
        provider = new GitHubCopilotProvider(instanceId)
        break
      }
      case "antigravity": {
        const { AntigravityProvider } = await import("~/providers/antigravity")
        provider = new AntigravityProvider(instanceId)
        break
      }
      case "alibaba": {
        const { AlibabaProvider } = await import("~/providers/alibaba")
        provider = new AlibabaProvider(instanceId)
        break
      }
      case "azure-openai": {
        const { AzureOpenAIProvider } = await import("~/providers/azure-openai")
        provider = new AzureOpenAIProvider(instanceId)
        break
      }
      default: {
        return c.json(
          { error: `Provider type ${providerType} not supported` },
          400,
        )
      }
    }

    // Register the new provider instance
    await providerRegistry.register(provider)

    consola.info(
      `Created new provider instance: ${provider.name} (${instanceId})`,
    )

    return c.json({
      success: true,
      provider: {
        id: provider.instanceId,
        type: provider.id,
        name: provider.name,
        isActive: false,
        authStatus: "unauthenticated",
        enabledModelCount: 0,
        totalModelCount: 0,
      },
    })
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    consola.error(`Failed to create provider instance:`, message)
    return c.json(
      { error: `Failed to create provider instance: ${message}` },
      500,
    )
  }
}

// ─── PUT /api/admin/providers/:id/config ──────────────────────────────────────

export async function handleUpdateProviderConfig(c: Context) {
  const instanceId = c.req.param("id")

  if (!providerRegistry.isRegistered(instanceId)) {
    return c.json({ error: `Unknown provider: ${instanceId}` }, 400)
  }

  const provider = providerRegistry.getProvider(instanceId)
  const body = await c.req.json<Record<string, any>>()

  try {
    if (provider.id === "azure-openai") {
      const { endpoint, apiVersion, deployments } = body

      if (!endpoint?.trim()) {
        return c.json({ error: "endpoint is required" }, 400)
      }
      if (!deployments || !Array.isArray(deployments)) {
        return c.json({ error: "deployments array is required" }, 400)
      }

      const { normalizeEndpoint, resourceNameFromEndpoint } = await import(
        "~/providers/azure-openai/auth"
      )
      const { AZURE_OPENAI_DEFAULT_API_VERSION } = await import(
        "~/providers/azure-openai/constants"
      )

      const normalizedEndpoint = normalizeEndpoint(endpoint.trim())
      const resource_name = resourceNameFromEndpoint(normalizedEndpoint)
      const deploymentsArray = deployments
        .filter((d) => d && typeof d === "string" && d.trim())
        .map((d) => d.trim())

      if (deploymentsArray.length === 0) {
        return c.json({ error: "At least one deployment is required" }, 400)
      }

      const config = {
        endpoint: normalizedEndpoint,
        api_version: apiVersion?.trim() || AZURE_OPENAI_DEFAULT_API_VERSION,
        resource_name,
        deployments: deploymentsArray,
      }

      // Save to configuration database
      saveAzureOpenAIConfig(instanceId, config)

      // Also update token data so provider uses new deployments immediately
      const existingToken = await readAzureOpenAIToken(instanceId)
      if (existingToken) {
        await writeAzureOpenAIToken(
          {
            ...existingToken,
            endpoint: normalizedEndpoint,
            api_version: apiVersion?.trim() || AZURE_OPENAI_DEFAULT_API_VERSION,
            resource_name,
            deployments: deploymentsArray,
          },
          instanceId,
        )

        // Refresh the provider to pick up new deployments
        try {
          await provider.setupAuth()
        } catch (err) {
          consola.warn(
            `Failed to refresh provider ${instanceId} after config update:`,
            err,
          )
        }
      }

      return c.json({ success: true, config })
    } else if (provider.id === "alibaba") {
      const { baseUrl, region } = body

      if (!baseUrl?.trim()) {
        return c.json({ error: "baseUrl is required" }, 400)
      }

      const config = {
        base_url: baseUrl.trim(),
        region: region || "global",
      }

      saveAlibabaConfig(instanceId, config)
      return c.json({ success: true, config })
    } else {
      return c.json(
        {
          error: `Configuration not supported for provider type: ${provider.id}`,
        },
        400,
      )
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    consola.error(
      `Failed to update provider config for ${instanceId}:`,
      message,
    )
    return c.json({ error: `Failed to update config: ${message}` }, 500)
  }
}

// ─── DELETE /api/admin/providers/:id ──────────────────────────────────────────

export async function handleDeleteProvider(c: Context) {
  const instanceId = c.req.param("id")

  try {
    const success = await providerRegistry.remove(instanceId)

    if (!success) {
      return c.json({ error: "Provider instance not found" }, 404)
    }

    // Clean up state and database
    state.disabledModels.delete(instanceId)
    state.providerModels.delete(instanceId)
    state.providerPriorities.delete(instanceId)

    // Clean up database records
    ProviderInstanceStore.delete(instanceId) // Delete provider instance record
    ModelStateStore.delete(instanceId) // Delete all model states for this instance
    ProviderConfigStore.delete(instanceId) // Delete config for this instance
    ModelConfigStore.delete(instanceId) // Delete all model configs for this instance

    // If this was the current provider, clear it
    if (state.currentProvider?.instanceId === instanceId) {
      state.currentProvider = undefined
    }

    consola.info(`Deleted provider instance: ${instanceId}`)

    return c.json({
      success: true,
      message: `Provider instance ${instanceId} deleted successfully`,
    })
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    consola.error(`Failed to delete provider instance ${instanceId}:`, message)
    return c.json(
      { error: `Failed to delete provider instance: ${message}` },
      500,
    )
  }
}

// ─── PUT /api/admin/providers/:id/models/:modelId/version ─────────────────────

export async function handleSetModelVersion(c: Context) {
  const instanceId = c.req.param("id")
  const modelId = c.req.param("modelId")

  if (!providerRegistry.isRegistered(instanceId)) {
    return c.json({ error: `Unknown provider: ${instanceId}` }, 400)
  }

  try {
    const body = await c.req.json<{ version: string }>()
    const { version } = body

    if (!version || typeof version !== "string" || !version.trim()) {
      return c.json({ error: "version is required" }, 400)
    }

    // Set the model version
    ModelConfigStore.setVersion(instanceId, modelId, version.trim())

    // If this is an Azure OpenAI provider, also clear the model cache
    const provider = providerRegistry.getProvider(instanceId)
    if (provider.id === "azure-openai") {
      state.providerModels.delete(instanceId)
    }

    consola.info(`Set model version for ${instanceId}/${modelId} to ${version}`)

    return c.json({
      success: true,
      instanceId,
      modelId,
      version: version.trim(),
    })
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    consola.error(
      `Failed to set model version for ${instanceId}/${modelId}:`,
      message,
    )
    return c.json({ error: `Failed to set model version: ${message}` }, 500)
  }
}

// ─── GET /api/admin/providers/:id/models/:modelId/version ─────────────────────

export async function handleGetModelVersion(c: Context) {
  const instanceId = c.req.param("id")
  const modelId = c.req.param("modelId")

  if (!providerRegistry.isRegistered(instanceId)) {
    return c.json({ error: `Unknown provider: ${instanceId}` }, 400)
  }

  try {
    const version = ModelConfigStore.getVersion(instanceId, modelId)

    return c.json({
      instanceId,
      modelId,
      version,
    })
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    consola.error(
      `Failed to get model version for ${instanceId}/${modelId}:`,
      message,
    )
    return c.json({ error: `Failed to get model version: ${message}` }, 500)
  }
}
