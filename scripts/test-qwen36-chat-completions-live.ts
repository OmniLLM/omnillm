#!/usr/bin/env bun

import process from "node:process"

const DEFAULT_BASE_URL = "http://127.0.0.1:5000"
const DEFAULT_MODEL = "qwen3.6-plus"
const DEFAULT_TIMEOUT_MS = 60_000

type ChatCompletionToolCall = {
  id?: string
  type?: string
  function?: {
    name?: string
    arguments?: string
  }
}

type ChatCompletionResponse = {
  choices?: Array<{
    finish_reason?: string | null
    message?: {
      content?: string | null
      tool_calls?: Array<ChatCompletionToolCall>
    }
  }>
}

function parsePositiveInt(value: string | undefined, fallback: number): number {
  if (!value) {
    return fallback
  }

  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

function authHeaders(apiKey: string): Record<string, string> {
  const trimmed = apiKey.trim()
  return trimmed === ""
    ? {}
    : {
        Authorization: `Bearer ${trimmed}`,
        "x-api-key": trimmed,
      }
}

async function verifyModel(
  baseUrl: string,
  apiKey: string,
  model: string,
): Promise<void> {
  const response = await fetch(`${baseUrl}/v1/models`, {
    headers: authHeaders(apiKey),
  })
  if (!response.ok) {
    throw new Error(`GET /v1/models failed with ${response.status}`)
  }

  const payload = (await response.json()) as {
    data?: Array<{ id?: string }>
  }
  const modelIDs =
    payload.data
      ?.map((entry) => entry.id)
      .filter((entry): entry is string => typeof entry === "string") ?? []

  if (!modelIDs.includes(model)) {
    throw new Error(
      `Model ${model} was not returned by /v1/models. Available models: ${modelIDs.join(", ") || "(none)"}`,
    )
  }
}

async function postChatCompletion(
  baseUrl: string,
  apiKey: string,
  payload: Record<string, unknown>,
): Promise<ChatCompletionResponse> {
  const response = await fetch(`${baseUrl}/v1/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...authHeaders(apiKey),
    },
    body: JSON.stringify(payload),
  })

  const text = await response.text()
  if (!response.ok) {
    throw new Error(`POST /v1/chat/completions failed with ${response.status}: ${text}`)
  }

  try {
    return JSON.parse(text) as ChatCompletionResponse
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error)
    throw new Error(`Invalid JSON from /v1/chat/completions: ${message}\n${text}`)
  }
}

function requireChoice(
  response: ChatCompletionResponse,
  label: string,
): NonNullable<ChatCompletionResponse["choices"]>[number] {
  const choice = response.choices?.[0]
  if (!choice) {
    throw new Error(`${label}: response did not contain choices[0]`)
  }
  return choice
}

async function main(): Promise<void> {
  const baseUrl = process.env.OMNILLM_OPENAI_BASE_URL ?? DEFAULT_BASE_URL
  const apiKey = process.env.OMNILLM_OPENAI_API_KEY ?? "sk-omnillm-local-test"
  const model = process.env.OMNILLM_OPENAI_MODEL ?? DEFAULT_MODEL
  const timeoutMs = parsePositiveInt(
    process.env.OMNILLM_OPENAI_TIMEOUT_MS,
    DEFAULT_TIMEOUT_MS,
  )

  console.log(
    `Running live chat/completions tool-loop check against ${baseUrl} with model ${model}`,
  )

  const abortController = new AbortController()
  const timeout = setTimeout(() => abortController.abort(), timeoutMs)

  try {
    await verifyModel(baseUrl, apiKey, model)
    console.log(`Verified that ${model} is exposed by ${baseUrl}/v1/models`)

    const tools = [
      {
        type: "function",
        function: {
          name: "Read",
          description: "Read a repository file and return its content",
          parameters: {
            type: "object",
            properties: {
              file_path: { type: "string" },
            },
            required: ["file_path"],
          },
        },
      },
    ]

    const firstResponse = await fetch(`${baseUrl}/v1/chat/completions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders(apiKey),
      },
      body: JSON.stringify({
        model,
        max_tokens: 256,
        messages: [
          {
            role: "user",
            content:
              "Read README.md first, then explain this gateway briefly. Use the Read tool before answering.",
          },
        ],
        tools,
      }),
      signal: abortController.signal,
    })

    const firstText = await firstResponse.text()
    if (!firstResponse.ok) {
      throw new Error(
        `First POST /v1/chat/completions failed with ${firstResponse.status}: ${firstText}`,
      )
    }

    let firstPayload: ChatCompletionResponse
    try {
      firstPayload = JSON.parse(firstText) as ChatCompletionResponse
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error)
      throw new Error(`Invalid first-turn JSON: ${message}\n${firstText}`)
    }

    const firstChoice = requireChoice(firstPayload, "first turn")
    if (firstChoice.finish_reason !== "tool_calls") {
      throw new Error(
        `First turn finish_reason was ${firstChoice.finish_reason ?? "null"}, expected tool_calls`,
      )
    }

    const firstToolCall = firstChoice.message?.tool_calls?.[0]
    if (!firstToolCall?.id || firstToolCall.type !== "function") {
      throw new Error(`First turn did not contain a valid tool call: ${firstText}`)
    }
    if (firstToolCall.function?.name !== "Read") {
      throw new Error(
        `First turn tool name was ${firstToolCall.function?.name ?? "null"}, expected Read`,
      )
    }

    let toolArguments: { file_path?: string }
    try {
      toolArguments = JSON.parse(firstToolCall.function.arguments ?? "{}") as {
        file_path?: string
      }
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : String(error)
      throw new Error(
        `Could not parse first turn tool arguments: ${message}\n${firstToolCall.function?.arguments ?? ""}`,
      )
    }

    if (!toolArguments.file_path) {
      throw new Error(`First turn tool arguments omitted file_path: ${firstText}`)
    }
    console.log(
      `Observed tool call ${firstToolCall.id}: Read(${toolArguments.file_path})`,
    )

    const toolResultContent =
      toolArguments.file_path === "README.md"
        ? "README.md says OmniLLM is a unified LLM gateway that normalizes requests into CIF and exposes OpenAI-compatible plus Anthropic-compatible APIs."
        : `${toolArguments.file_path} says OmniLLM routes provider traffic through a shared CIF core.`

    const secondPayload = await postChatCompletion(baseUrl, apiKey, {
      model,
      max_tokens: 256,
      messages: [
        {
          role: "user",
          content:
            "Read README.md first, then explain this gateway briefly. Use the Read tool before answering.",
        },
        {
          role: "assistant",
          content: firstChoice.message?.content ?? "",
          tool_calls: firstChoice.message?.tool_calls ?? [],
        },
        {
          role: "tool",
          tool_call_id: firstToolCall.id,
          content: toolResultContent,
        },
        {
          role: "user",
          content:
            "Finish the explanation in 2-4 sentences and mention CIF explicitly.",
        },
      ],
      tools,
    })

    const secondChoice = requireChoice(secondPayload, "second turn")
    if (secondChoice.finish_reason !== "stop") {
      throw new Error(
        `Second turn finish_reason was ${secondChoice.finish_reason ?? "null"}, expected stop`,
      )
    }
    if ((secondChoice.message?.tool_calls?.length ?? 0) > 0) {
      throw new Error("Second turn unexpectedly returned another tool call")
    }

    const finalText = secondChoice.message?.content?.trim() ?? ""
    if (finalText.length < 80) {
      throw new Error(`Final answer was too short (${finalText.length} chars): ${finalText}`)
    }

    const detailSignals = ["cif", "openai", "anthropic", "gateway", "provider"]
    const signalMatches = detailSignals.filter((signal) =>
      finalText.toLowerCase().includes(signal),
    )
    if (signalMatches.length < 2) {
      throw new Error(
        `Final answer did not contain enough expected detail (${signalMatches.length} matches): ${finalText}`,
      )
    }

    console.log("")
    console.log("Final answer")
    console.log(finalText)
    console.log("")
    console.log("Live chat/completions tool-loop check passed.")
  } finally {
    clearTimeout(timeout)
  }
}

await main()
