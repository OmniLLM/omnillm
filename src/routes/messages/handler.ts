import type { Context } from "hono"

import consola from "consola"
import { events } from "fetch-event-stream"
import { streamSSE } from "hono/streaming"

// CIF imports
import { parseAnthropicMessages } from "~/ingestion/from-anthropic"
import { awaitApproval } from "~/lib/approval"
import { isModelNotSupportedError, isUnsupportedApiForModel } from "~/lib/error"
import { checkRateLimit } from "~/lib/rate-limit"
import { resolveProvidersForModel } from "~/lib/model-routing"
import { state } from "~/lib/state"
import { providerRegistry } from "~/providers/registry"
import {
  serializeToAnthropic,
  cifEventToAnthropicSSE,
  createAnthropicStreamState,
} from "~/serialization"
import {
  type ChatCompletionChunk,
  type ChatCompletionResponse,
} from "~/services/copilot/create-chat-completions"

import {
  type AnthropicMessagesPayload,
  type AnthropicStreamState,
} from "./anthropic-types"
import {
  translateToAnthropic,
  translateToOpenAI,
} from "./non-stream-translation"
import { translateChunkToAnthropicEvents } from "./stream-translation"

// Helper function to normalize model names for validation
function normalizeModelName(modelName: string): string {
  // Handle Claude model variants with date suffixes
  const claudeModelMap: Record<string, string> = {
    "claude-haiku-4-5-20251001": "claude-haiku-4.5",
    "claude-sonnet-4-6-20241022": "claude-sonnet-4.6",
    "claude-opus-4-6-20241022": "claude-opus-4.6",
    "claude-3-5-sonnet-20241022": "claude-3.5-sonnet",
    "claude-3-5-sonnet-20240620": "claude-3.5-sonnet",
  }

  // Check exact mapping first
  if (claudeModelMap[modelName]) {
    return claudeModelMap[modelName]
  }

  // Handle pattern-based mappings
  if (modelName.startsWith("claude-haiku-4")) {
    return "claude-haiku-4.5"
  }
  if (modelName.startsWith("claude-sonnet-4-6")) {
    return "claude-sonnet-4.6"
  }
  if (modelName.startsWith("claude-sonnet-4.5")) {
    return "claude-sonnet-4.5"
  }
  if (modelName.startsWith("claude-sonnet-4")) {
    return "claude-sonnet-4"
  }
  if (modelName.startsWith("claude-opus-4")) {
    return "claude-opus-4.6"
  }

  return modelName
}

export async function handleCompletion(c: Context) {
  await checkRateLimit(state)

  const requestId = c.get("requestId")
  const startTime = Date.now()

  let anthropicPayload = await c.req.json<AnthropicMessagesPayload>()

  const requestedModel = anthropicPayload.model
  const normalizedModel = normalizeModelName(requestedModel)

  // Log REQUEST
  consola.info({
    type: "request",
    requestId,
    apiShape: "anthropic",
    modelRequested: requestedModel,
    modelNormalized: normalizedModel,
    messages: anthropicPayload.messages?.length ?? 0,
    tools: anthropicPayload.tools?.length ?? 0,
    stream: anthropicPayload.stream ?? false,
    maxTokens: anthropicPayload.max_tokens,
    hasSystemPrompt: Boolean(anthropicPayload.system),
  })

  const resolvedRoute = await resolveProvidersForModel(
    requestedModel,
    normalizedModel,
  )

  if (!resolvedRoute) {
    consola.warn("⚠️ No active providers available")
    return c.json(
      {
        type: "error",
        error: {
          type: "invalid_request_error",
          message: "No active providers configured",
        },
      },
      400,
    )
  }

  const { availableModels, candidateProviders, selectedModel } = resolvedRoute

  // Validate that the requested model exists (either original or normalized form)
  if (!selectedModel) {
    const availableModelIds =
      availableModels.map((m) => m.id).join(", ") || "none"
    consola.warn(
      `⚠️ Model '${requestedModel}' not found in loaded models. Available models: ${availableModelIds}`,
    )

    return c.json(
      {
        type: "error",
        error: {
          type: "invalid_request_error",
          message: `Model '${requestedModel}' not found. Available models: [${availableModelIds}]`,
        },
      },
      400,
    )
  }

  const tryProviders = candidateProviders

  // Update the payload to use the normalized model name for provider processing
  if (normalizedModel !== requestedModel) {
    consola.debug(
      `[Messages] Model normalized: ${requestedModel} -> ${normalizedModel}`,
    )
    anthropicPayload = { ...anthropicPayload, model: normalizedModel }
  }

  // Translate with no provider override — just for logging purposes
  const openAIPayload = translateToOpenAI(anthropicPayload)
  const modelMap =
    anthropicPayload.model !== openAIPayload.model ?
      `${anthropicPayload.model} → ${openAIPayload.model}`
    : anthropicPayload.model
  consola.info(
    `📥 POST /v1/messages → OpenAI chat/completions | Model: ${modelMap}`,
  )

  consola.debug(
    `Translated payload: ${openAIPayload.messages?.length ?? 0} messages, tools: ${openAIPayload.tools?.length ?? 0}, max_tokens: ${openAIPayload.max_tokens}`,
  )

  if (state.manualApprove) {
    await awaitApproval()
  }

  if (tryProviders.length === 0) {
    throw new Error(
      "No active providers available. Please activate a provider through the admin interface.",
    )
  }

  let firstError: unknown
  let firstRelevantError: unknown

  // Try each provider in priority order
  for (const tryProvider of tryProviders) {
    try {
      // Try CIF path first if adapter is available
      if (tryProvider.adapter) {
        consola.debug(
          `Using CIF adapter for ${tryProvider.name} (${tryProvider.instanceId})`,
        )

        const canonicalReq = parseAnthropicMessages(anthropicPayload)

        // Apply model remapping if needed
        const remappedModel =
          tryProvider.adapter.remapModel?.(canonicalReq.model)
          ?? canonicalReq.model
        const finalCanonicalReq = { ...canonicalReq, model: remappedModel }

        if (canonicalReq.stream) {
          consola.debug("Streaming response via CIF adapter")
          return streamSSE(c, async (stream) => {
            const cifStreamState = createAnthropicStreamState()

            for await (const cifEvent of tryProvider.adapter!.executeStream(
              finalCanonicalReq,
            )) {
              const anthEvents = cifEventToAnthropicSSE(
                cifEvent,
                cifStreamState,
              )
              for (const event of anthEvents) {
                await stream.writeSSE({
                  event: event.type,
                  data: JSON.stringify(event),
                })
              }
            }

            // Log RESPONSE
            consola.info({
              type: "response",
              requestId,
              apiShape: "anthropic",
              modelRequested: requestedModel,
              modelUsed: finalCanonicalReq.model,
              provider: `${tryProvider.name} (${tryProvider.instanceId})`,
              stream: true,
              latencyMs: Date.now() - startTime,
            })
          })
        } else {
          const canonicalResp =
            await tryProvider.adapter.execute(finalCanonicalReq)
          const anthropicResponse = serializeToAnthropic(canonicalResp)

          // Log RESPONSE
          consola.info({
            type: "response",
            requestId,
            apiShape: "anthropic",
            modelRequested: requestedModel,
            modelUsed: canonicalResp.model,
            provider: `${tryProvider.name} (${tryProvider.instanceId})`,
            stopReason: canonicalResp.stopReason,
            stream: false,
            inputTokens: canonicalResp.usage?.inputTokens,
            outputTokens: canonicalResp.usage?.outputTokens,
            latencyMs: Date.now() - startTime,
          })

          return c.json(anthropicResponse)
        }
      }

      // Fallback to existing translation path
      const providerPayload = translateToOpenAI(
        anthropicPayload,
        tryProvider.id,
      )
      consola.debug(
        `Trying ${tryProvider.name} (${tryProvider.instanceId}) for model ${providerPayload.model} (legacy path)`,
      )

      const response = await tryProvider.createChatCompletions(
        providerPayload as unknown as Record<string, unknown>,
      )
      if (providerPayload.stream) {
        consola.debug("Streaming response from provider")
        return streamSSE(c, async (stream) => {
          const streamState: AnthropicStreamState = {
            messageStartSent: false,
            contentBlockIndex: 0,
            contentBlockOpen: false,
            toolCalls: {},
          }
          for await (const rawEvent of events(response)) {
            consola.debug(
              "Provider raw stream event:",
              JSON.stringify(rawEvent),
            )
            if (rawEvent.data === "[DONE]") break
            if (!rawEvent.data) continue
            const chunk = JSON.parse(rawEvent.data) as ChatCompletionChunk
            const anthEvents = translateChunkToAnthropicEvents(
              chunk,
              streamState,
            )
            for (const event of anthEvents) {
              await stream.writeSSE({
                event: event.type,
                data: JSON.stringify(event),
              })
            }
          }

          // Log RESPONSE
          consola.info({
            type: "response",
            requestId,
            apiShape: "anthropic",
            modelRequested: requestedModel,
            modelUsed: providerPayload.model,
            provider: `${tryProvider.name} (${tryProvider.instanceId})`,
            stream: true,
            latencyMs: Date.now() - startTime,
          })
        })
      }
      const json = (await response.json()) as ChatCompletionResponse
      const anthropicResponse = translateToAnthropic(json)

      // Log RESPONSE
      consola.info({
        type: "response",
        requestId,
        apiShape: "anthropic",
        modelRequested: requestedModel,
        modelUsed: json.model,
        provider: `${tryProvider.name} (${tryProvider.instanceId})`,
        stopReason: json.choices?.[0]?.finish_reason,
        stream: false,
        inputTokens: json.usage?.prompt_tokens,
        outputTokens: json.usage?.completion_tokens,
        latencyMs: Date.now() - startTime,
      })

      return c.json(anthropicResponse)
    } catch (err) {
      // Get model name for error reporting (either from CIF or legacy path)
      const modelName =
        tryProvider.adapter ?
          anthropicPayload.model
        : translateToOpenAI(anthropicPayload, tryProvider.id).model

      firstError ??= err
      if (!firstRelevantError && !(await isModelNotSupportedError(err))) {
        firstRelevantError = err
      }

      if (await isUnsupportedApiForModel(err)) {
        consola.info(
          `${tryProvider.name} (${tryProvider.instanceId}) does not support /chat/completions, trying next provider`,
        )
      } else {
        const errorMsg = err instanceof Error ? err.message : String(err)
        consola.warn(
          `${tryProvider.name} (${tryProvider.instanceId}) failed for model ${modelName}, trying next provider: ${errorMsg}`,
        )
      }
      continue
    }
  }

  throw (
    firstRelevantError
    ?? firstError
    ?? new Error(
      `All active providers failed for model ${anthropicPayload.model}`,
    )
  )
}
