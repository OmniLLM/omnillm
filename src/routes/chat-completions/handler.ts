import type { Context } from "hono"

import consola from "consola"
import { events } from "fetch-event-stream"
import { streamSSE, type SSEMessage } from "hono/streaming"

import type { ResponsesResponse } from "~/routes/responses/types"
import type {
  ChatCompletionChunk,
  ChatCompletionResponse,
} from "~/services/copilot/create-chat-completions"

// CIF imports
import { parseOpenAIChatCompletions } from "~/ingestion/from-openai"
import { parseResponsesPayload } from "~/ingestion/from-responses"
import { awaitApproval } from "~/lib/approval"
import {
  isModelNotSupportedError,
  isUnsupportedApiForModel,
  shouldFallbackToResponsesAPI,
} from "~/lib/error"
import { checkRateLimit } from "~/lib/rate-limit"
import { resolveProvidersForModel } from "~/lib/model-routing"
import { state } from "~/lib/state"
import { getTokenCount } from "~/lib/tokenizer"
import { isNullish } from "~/lib/utils"
import { AZURE_OPENAI_DEFAULT_MAX_OUTPUT_TOKENS } from "~/providers/azure-openai/constants"
import { providerRegistry } from "~/providers/registry"
import {
  chatCompletionsToResponsesPayload,
  responsesResponseToChatCompletions,
  responsesEventToChatChunk,
  initStreamState,
  translateChunkToResponsesEvents,
  translateResponseToResponses,
} from "~/routes/responses/translation"
import {
  serializeToOpenAI,
  cifEventToOpenAISSE,
  createOpenAIStreamState,
} from "~/serialization"
import {
  serializeToResponses,
  cifEventToResponsesSSE,
  createResponsesStreamState,
} from "~/serialization"
import { type ChatCompletionsPayload } from "~/services/copilot/create-chat-completions"

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

function getDefaultMaxTokensForModel(model: {
  vendor?: string
  capabilities?: {
    limits?: {
      max_output_tokens?: number
    }
  }
}): number | undefined {
  if (model.vendor === "azure-openai") {
    return Math.min(
      model.capabilities?.limits?.max_output_tokens
        ?? AZURE_OPENAI_DEFAULT_MAX_OUTPUT_TOKENS,
      AZURE_OPENAI_DEFAULT_MAX_OUTPUT_TOKENS,
    )
  }

  return model.capabilities?.limits?.max_output_tokens
}

async function executeResponsesFallback(
  c: Context,
  provider: ReturnType<typeof providerRegistry.getActiveProviders>[number],
  payload: ChatCompletionsPayload,
  requestId: string,
  requestedModel: string,
  startTime: number,
) {
  const responsesPayload = chatCompletionsToResponsesPayload(payload)

  if (provider.adapter) {
    const canonicalReq = parseResponsesPayload(responsesPayload)
    const remappedModel =
      provider.adapter.remapModel?.(canonicalReq.model) ?? canonicalReq.model
    const finalCanonicalReq = { ...canonicalReq, model: remappedModel }

    if (canonicalReq.stream) {
      return streamSSE(c, async (stream) => {
        const cifStreamState = createResponsesStreamState()
        const chunkId = crypto.randomUUID()
        const created = Math.floor(Date.now() / 1000)

        for await (const cifEvent of provider.adapter.executeStream(
          finalCanonicalReq,
        )) {
          const responsesEvents = cifEventToResponsesSSE(
            cifEvent,
            cifStreamState,
          )
          for (const event of responsesEvents) {
            const chunk = responsesEventToChatChunk(event, {
              model: finalCanonicalReq.model,
              chunkId,
              created,
            })
            if (chunk) {
              await stream.writeSSE({ data: JSON.stringify(chunk) })
            }
          }
        }
        await stream.writeSSE({ data: "[DONE]" })

        // Log RESPONSE
        consola.info({
          type: "response",
          requestId,
          apiShape: "openai",
          modelRequested: requestedModel,
          modelUsed: finalCanonicalReq.model,
          provider: `${provider.name} (${provider.instanceId})`,
          stream: true,
          latencyMs: Date.now() - startTime,
        })
      })
    }

    const canonicalResp = await provider.adapter.execute(finalCanonicalReq)
    const responsesResponse = serializeToResponses(canonicalResp)
    const chatResponse = responsesResponseToChatCompletions(
      responsesResponse as ResponsesResponse,
    )

    // Log RESPONSE
    consola.info({
      type: "response",
      requestId,
      apiShape: "openai",
      modelRequested: requestedModel,
      modelUsed: canonicalResp.model,
      provider: `${provider.name} (${provider.instanceId})`,
      stopReason: canonicalResp.stopReason,
      stream: false,
      inputTokens: canonicalResp.usage?.inputTokens,
      outputTokens: canonicalResp.usage?.outputTokens,
      latencyMs: Date.now() - startTime,
    })

    return c.json(chatResponse)
  }

  const fallbackOpenAIPayload = chatCompletionsToResponsesPayload(payload)
  // eslint-disable-next-line @typescript-eslint/no-deprecated
  const response = await provider.createChatCompletions(
    fallbackOpenAIPayload as unknown as Record<string, unknown>,
  )

  if (responsesPayload.stream) {
    return streamSSE(c, async (stream) => {
      const streamState = initStreamState()
      const chunkId = crypto.randomUUID()
      const created = Math.floor(Date.now() / 1000)

      for await (const rawEvent of events(response)) {
        if (rawEvent.data === "[DONE]") {
          break
        }
        if (!rawEvent.data) {
          continue
        }

        const chunk = JSON.parse(rawEvent.data) as ChatCompletionChunk
        const responsesEvents = translateChunkToResponsesEvents(
          chunk,
          streamState,
        )
        for (const event of responsesEvents) {
          const chatChunk = responsesEventToChatChunk(event, {
            model: responsesPayload.model,
            chunkId,
            created,
          })
          if (chatChunk) {
            await stream.writeSSE({ data: JSON.stringify(chatChunk) })
          }
        }
      }
      await stream.writeSSE({ data: "[DONE]" })

      // Log RESPONSE
      consola.info({
        type: "response",
        requestId,
        apiShape: "openai",
        modelRequested: requestedModel,
        modelUsed: responsesPayload.model,
        provider: `${provider.name} (${provider.instanceId})`,
        stream: true,
        latencyMs: Date.now() - startTime,
      })
    })
  }

  const json = (await response.json()) as ChatCompletionResponse
  const responsesResponse = translateResponseToResponses(json)
  const chatResponse = responsesResponseToChatCompletions(responsesResponse)

  // Log RESPONSE
  consola.info({
    type: "response",
    requestId,
    apiShape: "openai",
    modelRequested: requestedModel,
    modelUsed: json.model,
    provider: `${provider.name} (${provider.instanceId})`,
    stopReason: json.choices?.[0]?.finish_reason,
    stream: false,
    inputTokens: json.usage?.prompt_tokens,
    outputTokens: json.usage?.completion_tokens,
    latencyMs: Date.now() - startTime,
  })

  return c.json(chatResponse)
}

/* eslint-disable max-lines-per-function, complexity */
export async function handleCompletion(c: Context) {
  await checkRateLimit(state)

  const requestId = c.get("requestId")
  const startTime = Date.now()

  let payload = await c.req.json<ChatCompletionsPayload>()
  const requestedModel = payload.model
  const normalizedModel = normalizeModelName(requestedModel)

  // Log REQUEST
  consola.info({
    type: "request",
    requestId,
    apiShape: "openai",
    modelRequested: requestedModel,
    modelNormalized: normalizedModel,
    messages: payload.messages?.length ?? 0,
    tools: payload.tools?.length ?? 0,
    stream: payload.stream ?? false,
    maxTokens: payload.max_tokens,
  })

  consola.debug(
    `Request summary: model=${payload.model}, messages=${payload.messages?.length ?? 0}, tools=${payload.tools?.length ?? 0}, stream=${payload.stream ?? false}, max_tokens=${payload.max_tokens}`,
  )
  consola.info(`📤 Model called: ${payload.model}`)

  // Validate and filter tools: only include if they have all required fields
  if (payload.tools && payload.tools.length > 0) {
    const validTools = payload.tools.filter(
      (tool) =>
        Boolean(tool.function.name) && Boolean(tool.function.parameters),
    )
    if (validTools.length === 0) {
      consola.warn("All tools were filtered out due to missing required fields")
      payload = { ...payload, tools: undefined }
    } else if (validTools.length < payload.tools.length) {
      consola.warn(
        `Filtered tools: ${validTools.length}/${payload.tools.length} tools have complete definitions`,
      )
      payload = { ...payload, tools: validTools }
    }
  }

  const requestedModel = payload.model
  const normalizedModel = normalizeModelName(requestedModel)

  const resolvedRoute = await resolveProvidersForModel(
    requestedModel,
    normalizedModel,
  )

  if (!resolvedRoute) {
    consola.warn("⚠️ No active providers available")
    return c.json(
      {
        error: {
          message: "No active providers configured",
          type: "invalid_request_error",
          code: "no_providers",
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

    // Return proper error response for unavailable model
    return c.json(
      {
        error: {
          message: `Model '${requestedModel}' not found. Available models: [${availableModelIds}]`,
          type: "invalid_request_error",
          param: "model",
          code: "model_not_found",
        },
      },
      400,
    )
  }

  // Update the payload to use the normalized model name for provider processing
  if (normalizedModel !== requestedModel) {
    consola.debug(
      `[Chat Completions] Model normalized: ${requestedModel} -> ${normalizedModel}`,
    )
    payload = { ...payload, model: normalizedModel }
  }

  const maxTokens = selectedModel.capabilities?.limits?.max_output_tokens
  if (maxTokens) {
    consola.info(
      `✅ Model details - Name: ${selectedModel.id}, Max tokens: ${maxTokens}`,
    )
  }

  // Calculate and display token count
  try {
    const tokenCount = await getTokenCount(payload, selectedModel)
    consola.info("Current token count:", tokenCount)
  } catch (error) {
    consola.warn("Failed to calculate token count:", error)
  }

  if (state.manualApprove) await awaitApproval()

  if (isNullish(payload.max_tokens)) {
    const defaultMaxTokens = getDefaultMaxTokensForModel(selectedModel)
    if (defaultMaxTokens) {
      payload = {
        ...payload,
        max_tokens: defaultMaxTokens,
      }
      consola.debug("Set max_tokens to:", payload.max_tokens)
    }
  }

  const tryProviders = candidateProviders

  if (tryProviders.length === 0) {
    return c.json(
      {
        error: {
          type: "provider_error",
          message:
            "No active providers available. Please activate a provider through the admin interface.",
        },
      },
      503,
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
          `[CIF Path] Using CIF adapter for ${tryProvider.name} (${tryProvider.instanceId})`,
        )

        const canonicalReq = parseOpenAIChatCompletions(payload)
        consola.debug(
          `[CIF Path] Converted to canonical format: ${canonicalReq.messages.length} messages, stream: ${canonicalReq.stream}`,
        )

        // Apply model remapping if needed
        const remappedModel =
          tryProvider.adapter.remapModel?.(canonicalReq.model)
          ?? canonicalReq.model
        const finalCanonicalReq = { ...canonicalReq, model: remappedModel }

        if (remappedModel !== canonicalReq.model) {
          consola.debug(
            `[CIF Path] Model remapped: ${canonicalReq.model} -> ${remappedModel}`,
          )
        }

        if (canonicalReq.stream) {
          consola.debug("[CIF Path] Streaming response via CIF adapter")
          return streamSSE(c, async (stream) => {
            const cifStreamState = createOpenAIStreamState()

            for await (const cifEvent of tryProvider.adapter.executeStream(
              finalCanonicalReq,
            )) {
              const openAIChunk = cifEventToOpenAISSE(cifEvent, cifStreamState)
              if (openAIChunk) {
                await stream.writeSSE({
                  data: JSON.stringify(openAIChunk),
                })
              }
            }
            // Send final [DONE] message
            await stream.writeSSE({
              data: "[DONE]",
            })
          })
        } else {
          consola.debug("[CIF Path] Non-streaming response via CIF adapter")
          const canonicalResp =
            await tryProvider.adapter.execute(finalCanonicalReq)
          consola.debug(
            `[CIF Path] Received canonical response: ${canonicalResp.content.length} content parts, stop reason: ${canonicalResp.stopReason}`,
          )
          const openAIResponse = serializeToOpenAI(canonicalResp)

          // Log RESPONSE
          consola.info({
            type: "response",
            requestId,
            apiShape: "openai",
            modelRequested: requestedModel,
            modelUsed: canonicalResp.model,
            provider: `${tryProvider.name} (${tryProvider.instanceId})`,
            stopReason: canonicalResp.stopReason,
            stream: false,
            inputTokens: canonicalResp.usage?.inputTokens,
            outputTokens: canonicalResp.usage?.outputTokens,
            latencyMs: Date.now() - startTime,
          })

          return c.json(openAIResponse)
        }
      }

      // Fallback to existing direct provider path
      consola.debug(
        `Trying ${tryProvider.name} (${tryProvider.instanceId}) for model ${payload.model} (legacy path)`,
      )
      // eslint-disable-next-line @typescript-eslint/no-deprecated
      const response = await tryProvider.createChatCompletions(
        payload as unknown as Record<string, unknown>,
      )
      if (payload.stream) {
        consola.debug("Streaming response via provider")
        return streamSSE(c, async (stream) => {
          for await (const chunk of events(response)) {
            consola.debug("Streaming chunk received")
            await stream.writeSSE(chunk as SSEMessage)
          }

          // Log RESPONSE
          consola.info({
            type: "response",
            requestId,
            apiShape: "openai",
            modelRequested: requestedModel,
            modelUsed: payload.model,
            provider: `${tryProvider.name} (${tryProvider.instanceId})`,
            stream: true,
            latencyMs: Date.now() - startTime,
          })
        })
      }
      const json = await response.json()
      consola.debug("Non-streaming response received")

      // Log RESPONSE
      consola.info({
        type: "response",
        requestId,
        apiShape: "openai",
        modelRequested: requestedModel,
        modelUsed: json.model,
        provider: `${tryProvider.name} (${tryProvider.instanceId})`,
        stopReason: json.choices?.[0]?.finish_reason,
        stream: false,
        inputTokens: json.usage?.prompt_tokens,
        outputTokens: json.usage?.completion_tokens,
        latencyMs: Date.now() - startTime,
      })

      return c.json(json)
    } catch (err) {
      if (await shouldFallbackToResponsesAPI(err)) {
        consola.info(
          `${tryProvider.name} (${tryProvider.instanceId}) rejected /chat/completions for ${payload.model}; falling back to Responses API`,
        )
        try {
          return await executeResponsesFallback(c, tryProvider, payload, requestId, requestedModel, startTime)
        } catch (fallbackErr) {
          firstError ??= fallbackErr
          if (
            !firstRelevantError
            && !(await isModelNotSupportedError(fallbackErr))
          ) {
            firstRelevantError = fallbackErr
          }

          const errorMsg =
            fallbackErr instanceof Error ?
              fallbackErr.message
            : String(fallbackErr)
          consola.warn(
            `${tryProvider.name} (${tryProvider.instanceId}) Responses fallback failed for model ${payload.model}, trying next provider: ${errorMsg}`,
          )
          continue
        }
      }

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
          `${tryProvider.name} (${tryProvider.instanceId}) failed for model ${payload.model}, trying next provider: ${errorMsg}`,
        )
      }
      continue
    }
  }

  throw (
    firstRelevantError
    ?? firstError
    ?? new Error(`All active providers failed for model ${payload.model}`)
  )
}
/* eslint-enable max-lines-per-function, complexity */
