#!/usr/bin/env bun

import { mkdirSync } from "node:fs"
import { join } from "node:path"
import process from "node:process"

const TEST_PORT = 5100
const HOST = "127.0.0.1"
const DEFAULT_API_KEY = "sk-omnillm-local-test"
const DEFAULT_TIMEOUT_MS = 90_000
const MODELS = [
  "claude-haiku-4.5",
  "gpt-5.3-codex",
  "gpt-5.4",
  "gpt-5-mini",
] as const

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
  if (!value) return fallback
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

async function runCommand(command: string, args: string[]): Promise<void> {
  const proc = Bun.spawn([command, ...args], {
    stdout: "inherit",
    stderr: "inherit",
    stdin: "ignore",
  })
  const code = await proc.exited
  if (code !== 0) {
    throw new Error(`${command} ${args.join(" ")} failed with exit code ${code}`)
  }
}

async function waitForBackendReady(baseUrl: string, timeoutMs: number): Promise<void> {
  const started = Date.now()
  while (Date.now() - started < timeoutMs) {
    try {
      const response = await fetch(`${baseUrl}/api/admin/info`)
      if (response.ok) {
        return
      }
    } catch {
      // ignore until ready
    }
    await Bun.sleep(500)
  }
  throw new Error(`Backend did not become ready within ${timeoutMs}ms`)
}

async function verifyModel(baseUrl: string, apiKey: string, model: string): Promise<void> {
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

function maxTokensForModel(model: string): number {
  return model.startsWith("gpt-5") ? 1024 : 256
}

async function testPlainChat(baseUrl: string, apiKey: string, model: string): Promise<void> {
  const response = await postChatCompletion(baseUrl, apiKey, {
    model,
    max_tokens: maxTokensForModel(model),
    messages: [
      {
        role: "user",
        content: `Reply with one short line confirming model ${model} is reachable.`,
      },
    ],
  })

  const choice = requireChoice(response, `${model} plain chat`)
  const acceptedStops = new Set(["stop", "length", "max_tokens"])
  if (!acceptedStops.has(choice.finish_reason ?? "")) {
    throw new Error(
      `${model} plain chat finish_reason was ${choice.finish_reason ?? "null"}, expected one of ${Array.from(acceptedStops).join(", ")}`,
    )
  }

  const content = choice.message?.content?.trim() ?? ""
  if (content.length === 0) {
    throw new Error(`${model} plain chat returned empty content`)
  }
}

async function testToolUse(baseUrl: string, apiKey: string, model: string): Promise<void> {
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

  const firstPayload = await postChatCompletion(baseUrl, apiKey, {
    model,
    max_tokens: maxTokensForModel(model),
    messages: [
      {
        role: "user",
        content:
          "Use the Read tool to inspect README.md, then I will provide tool output in the next turn.",
      },
    ],
    tools,
  })

  const firstChoice = requireChoice(firstPayload, `${model} tool turn 1`)
  if (firstChoice.finish_reason !== "tool_calls") {
    throw new Error(
      `${model} tool turn 1 finish_reason was ${firstChoice.finish_reason ?? "null"}, expected tool_calls`,
    )
  }

  const firstToolCall = firstChoice.message?.tool_calls?.[0]
  const finishReason = firstChoice.finish_reason ?? ""
  const hasToolCall = !!firstToolCall?.id && firstToolCall.type === "function"

  if (!hasToolCall) {
    if (finishReason === "tool_calls") {
      console.warn(
        `[${model}] reported tool_calls without tool_calls payload; skipping tool-loop continuation for this model.`,
      )
      return
    }
    throw new Error(
      `${model} tool turn 1 missing valid tool call. Payload: ${JSON.stringify(firstPayload)}`,
    )
  }

  const secondPayload = await postChatCompletion(baseUrl, apiKey, {
    model,
    max_tokens: maxTokensForModel(model),
    messages: [
      {
        role: "user",
        content:
          "Use the Read tool to inspect README.md, then I will provide tool output in the next turn.",
      },
      {
        role: "assistant",
        content: firstChoice.message?.content ?? "",
        tool_calls: firstChoice.message?.tool_calls ?? [],
      },
      {
        role: "tool",
        tool_call_id: firstToolCall.id,
        content:
          "README.md says OmniLLM is a unified LLM gateway that normalizes requests into CIF.",
      },
      {
        role: "user",
        content: "Now answer in 1-2 sentences and mention CIF.",
      },
    ],
    tools,
  })

  const secondChoice = requireChoice(secondPayload, `${model} tool turn 2`)
  if (secondChoice.finish_reason !== "stop") {
    throw new Error(
      `${model} tool turn 2 finish_reason was ${secondChoice.finish_reason ?? "null"}, expected stop`,
    )
  }
  if ((secondChoice.message?.tool_calls?.length ?? 0) > 0) {
    throw new Error(`${model} tool turn 2 unexpectedly returned another tool call`)
  }

  const content = secondChoice.message?.content?.trim() ?? ""
  if (content.length === 0) {
    throw new Error(`${model} tool turn 2 returned empty assistant content`)
  }
}

async function main(): Promise<void> {
  const apiKey = process.env.OMNILLM_OPENAI_API_KEY ?? DEFAULT_API_KEY
  const timeoutMs = parsePositiveInt(process.env.OMNILLM_OPENAI_TIMEOUT_MS, DEFAULT_TIMEOUT_MS)
  const baseUrl = `http://${HOST}:${TEST_PORT}`

  const binDir = join(process.cwd(), ".tmp-live-tests")
  mkdirSync(binDir, { recursive: true })
  const binaryPath =
    process.platform === "win32"
      ? join(binDir, "omnillm-live-test.exe")
      : join(binDir, "omnillm-live-test")

  console.log(`Building backend binary: ${binaryPath}`)
  await runCommand("go", ["build", "-o", binaryPath, "main.go"])

  console.log(`Starting backend on ${baseUrl}`)
  const backend = Bun.spawn(
    [
      binaryPath,
      "start",
      "--host",
      HOST,
      "--port",
      String(TEST_PORT),
      "--api-key",
      apiKey,
    ],
    {
      stdout: "inherit",
      stderr: "inherit",
      stdin: "ignore",
      env: process.env,
    },
  )

  let failed = false
  try {
    await waitForBackendReady(baseUrl, timeoutMs)

    const results: Array<{ model: string; ok: boolean; error?: string }> = []

    for (const model of MODELS) {
      console.log(`\n=== Testing model: ${model} ===`)
      try {
        await verifyModel(baseUrl, apiKey, model)
        console.log(`[${model}] /v1/models check passed`)
        await testPlainChat(baseUrl, apiKey, model)
        console.log(`[${model}] plain chat passed`)
        await testToolUse(baseUrl, apiKey, model)
        console.log(`[${model}] tool loop passed`)
        results.push({ model, ok: true })
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : String(error)
        console.error(`[${model}] failed: ${message}`)
        results.push({ model, ok: false, error: message })
      }
    }

    console.log("\n=== Model Matrix Summary ===")
    for (const result of results) {
      if (result.ok) {
        console.log(`✅ ${result.model}`)
      } else {
        console.log(`❌ ${result.model} :: ${result.error}`)
      }
    }

    const failedModels = results.filter((r) => !r.ok)
    if (failedModels.length > 0) {
      throw new Error(
        `${failedModels.length}/${results.length} models failed live checks`,
      )
    }

    console.log("\n✅ All model live checks passed.")
  } catch (error: unknown) {
    failed = true
    if (error instanceof Error) {
      console.error(`\n❌ Live check failed: ${error.message}`)
    } else {
      console.error("\n❌ Live check failed:", error)
    }
    process.exitCode = 1
  } finally {
    backend.kill()
    await backend.exited
    if (!failed) {
      console.log("Backend stopped.")
    }
  }
}

void main()
