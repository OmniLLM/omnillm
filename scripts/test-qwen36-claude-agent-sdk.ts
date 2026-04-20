#!/usr/bin/env bun

import {
  query,
  type SDKAssistantMessage,
  type SDKResultMessage,
} from "@anthropic-ai/claude-agent-sdk"
import { resolve } from "node:path"
import process from "node:process"

const scriptDir = import.meta.dirname
const repoRoot = resolve(scriptDir, "..")

const DEFAULT_BASE_URL = "http://127.0.0.1:5000"
const DEFAULT_MODEL = "qwen3.6-plus"
const DEFAULT_TIMEOUT_MS = 5 * 60 * 1000
const DEFAULT_MAX_TURNS = 20
const DEFAULT_ALLOWED_TOOLS = ["Glob", "Grep", "Read"]

const DEFAULT_PROMPT = `
Explain this codebase in detail after inspecting the repository directly.

Requirements:
- Use the available read-only tools first. Start by mapping the repo, then inspect concrete files before answering.
- Inspect representative files instead of exhaustively traversing every module.
- Do not answer from memory or assumptions.
- Cover architecture, request/response flow, provider abstractions, the frontend, and testing.
- Mention specific file paths you inspected.
- Explicitly cover both the Go backend under internal/ and the React frontend under frontend/.
- End with a short section on extension points and risks.
`.trim()

type ContentBlockLike = {
  type: string
  id?: string
  input?: unknown
  name?: string
  text?: string
}

type ToolUseObservation = {
  toolUseId: string
  toolName: string
  inputPreview: string
}

type RunSummary = {
  assistantText: Array<string>
  messageCount: number
  resultMessage: SDKResultMessage | null
  stderrLines: Array<string>
  streamEventCount: number
  toolCounts: Map<string, number>
  toolUses: Array<ToolUseObservation>
}

function parsePositiveInt(value: string | undefined, fallback: number): number {
  if (!value) {
    return fallback
  }

  const parsed = Number.parseInt(value, 10)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback
}

function parseToolList(value: string | undefined): Array<string> {
  if (!value) {
    return [...DEFAULT_ALLOWED_TOOLS]
  }

  const parsed = value
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0)

  return parsed.length > 0 ? parsed : [...DEFAULT_ALLOWED_TOOLS]
}

function truncate(text: string, maxLength: number): string {
  if (text.length <= maxLength) {
    return text
  }
  return `${text.slice(0, maxLength - 3)}...`
}

function preview(text: string): string {
  return truncate(text.replaceAll(/\s+/g, " ").trim(), 160)
}

function safeStringify(value: unknown): string {
  try {
    return JSON.stringify(value)
  } catch (error: unknown) {
    const message =
      error instanceof Error ? error.message : "unknown serialization error"
    return `[unserializable: ${message}]`
  }
}

function trackTool(toolCounts: Map<string, number>, toolName: string): void {
  toolCounts.set(toolName, (toolCounts.get(toolName) ?? 0) + 1)
}

function pushBounded(lines: Array<string>, value: string, maxLines = 60): void {
  lines.push(value)
  if (lines.length > maxLines) {
    lines.splice(0, lines.length - maxLines)
  }
}

function assistantBlocks(
  message: SDKAssistantMessage,
): Array<ContentBlockLike> {
  const { content } = message.message
  return Array.isArray(content) ? (content as Array<ContentBlockLike>) : []
}

function modelRequestHeaders(apiKey: string): Record<string, string> {
  const trimmedApiKey = apiKey.trim()
  if (trimmedApiKey === "") {
    return {}
  }

  return {
    Authorization: `Bearer ${trimmedApiKey}`,
    "anthropic-version": "2023-06-01",
    "x-api-key": trimmedApiKey,
  }
}

async function verifyBackendModel(
  baseUrl: string,
  model: string,
  apiKey: string,
): Promise<void> {
  const response = await fetch(`${baseUrl}/v1/models`, {
    headers: modelRequestHeaders(apiKey),
  })
  if (!response.ok) {
    throw new Error(`GET /v1/models failed with ${response.status}`)
  }

  const payload = (await response.json()) as {
    data?: Array<{ id?: string }>
  }

  const availableModels =
    payload.data
      ?.map((entry) => entry.id)
      .filter((entry): entry is string => typeof entry === "string") ?? []

  if (!availableModels.includes(model)) {
    throw new Error(
      `Model ${model} was not returned by /v1/models. Available models: ${availableModels.join(", ") || "(none)"}`,
    )
  }
}

function validateSummary(summary: RunSummary): Array<string> {
  const failures: Array<string> = []
  const resultMessage = summary.resultMessage

  if (resultMessage == null) {
    failures.push("No SDK result message was received.")
    return failures
  }

  if (resultMessage.subtype !== "success") {
    failures.push(`Result subtype was ${resultMessage.subtype}.`)
    return failures
  }

  if (resultMessage.is_error) {
    failures.push("SDK marked the run as an error.")
  }

  const terminalReason = resultMessage.terminal_reason ?? "completed"
  if (terminalReason !== "completed") {
    failures.push(`Terminal reason was ${terminalReason}.`)
  }

  if (resultMessage.stop_reason === "max_tokens") {
    failures.push("Model stopped because it hit max_tokens.")
  }

  if (resultMessage.permission_denials.length > 0) {
    failures.push(
      `Observed ${resultMessage.permission_denials.length} permission denials.`,
    )
  }

  if (summary.toolCounts.size === 0) {
    failures.push("No tool use was observed; expected repository inspection.")
  }

  const finalText = resultMessage.result.trim()
  if (finalText.length < 400) {
    failures.push(
      `Final explanation was too short (${finalText.length} characters).`,
    )
  }

  const detailSignals = [
    "internal/",
    "frontend/",
    "provider",
    "route",
    "test",
    "react",
    "go",
    "cif",
  ]
  const matchedSignals = detailSignals.filter((signal) =>
    finalText.toLowerCase().includes(signal),
  )
  if (matchedSignals.length < 3) {
    failures.push(
      `Final explanation did not contain enough concrete repo detail (${matchedSignals.length} signal matches).`,
    )
  }

  return failures
}

function printSummary(summary: RunSummary): void {
  const resultMessage = summary.resultMessage
  const finalText =
    resultMessage?.subtype === "success" ? resultMessage.result.trim() : ""

  console.log("")
  console.log("Summary")
  console.log(`- Messages observed: ${summary.messageCount}`)
  console.log(`- Stream events observed: ${summary.streamEventCount}`)
  console.log(`- Tool uses observed: ${summary.toolUses.length}`)
  console.log(
    `- Result subtype: ${resultMessage?.subtype ?? "missing"} | stop_reason: ${resultMessage?.stop_reason ?? "null"} | terminal_reason: ${resultMessage?.terminal_reason ?? "completed"}`,
  )
  console.log(`- Final explanation length: ${finalText.length} characters`)

  if (summary.toolCounts.size > 0) {
    console.log("- Tool counts:")
    for (const [toolName, count] of summary.toolCounts) {
      console.log(`  - ${toolName}: ${count}`)
    }
  }

  if (summary.toolUses.length > 0) {
    console.log("- Tool call samples:")
    for (const toolUse of summary.toolUses.slice(0, 8)) {
      console.log(
        `  - ${toolUse.toolName} (${toolUse.toolUseId}): ${toolUse.inputPreview}`,
      )
    }
  }

  if (finalText.length > 0) {
    console.log("")
    console.log("Final explanation")
    console.log(finalText)
  }
}

async function main(): Promise<void> {
  const baseUrl = process.env.OMNILLM_CLAUDE_BASE_URL ?? DEFAULT_BASE_URL
  const model = process.env.OMNILLM_CLAUDE_MODEL ?? DEFAULT_MODEL
  const apiKey =
    process.env.OMNILLM_CLAUDE_API_KEY ?? "sk-omnillm-local-test"
  const prompt = process.env.OMNILLM_CLAUDE_PROMPT ?? DEFAULT_PROMPT
  const timeoutMs = parsePositiveInt(
    process.env.OMNILLM_CLAUDE_TIMEOUT_MS,
    DEFAULT_TIMEOUT_MS,
  )
  const maxTurns = parsePositiveInt(
    process.env.OMNILLM_CLAUDE_MAX_TURNS,
    DEFAULT_MAX_TURNS,
  )
  const allowedTools = parseToolList(process.env.OMNILLM_CLAUDE_ALLOWED_TOOLS)
  const explicitlyAllowedToolNames = new Set(allowedTools)

  console.log(
    `Running Claude Agent SDK live check against ${baseUrl} with model ${model}`,
  )
  console.log(`Repo root: ${repoRoot}`)
  console.log(`Allowed Claude tools: ${allowedTools.join(", ")}`)

  await verifyBackendModel(baseUrl, model, apiKey)
  console.log(`Verified that ${model} is exposed by ${baseUrl}/v1/models`)

  const abortController = new AbortController()
  const timeout = setTimeout(() => {
    abortController.abort()
  }, timeoutMs)

  const summary: RunSummary = {
    assistantText: [],
    messageCount: 0,
    resultMessage: null,
    stderrLines: [],
    streamEventCount: 0,
    toolCounts: new Map<string, number>(),
    toolUses: [],
  }

  try {
    const messageStream = query({
      prompt,
      options: {
        abortController,
        allowedTools,
        cwd: repoRoot,
        env: {
          ...process.env,
          ANTHROPIC_API_KEY: apiKey,
          ANTHROPIC_BASE_URL: baseUrl,
          CLAUDE_AGENT_SDK_CLIENT_APP: "omnillm-qwen36-live-check/1.0.0",
        },
        includePartialMessages: true,
        maxTurns,
        model,
        canUseTool: async (toolName: string) => {
          if (explicitlyAllowedToolNames.has(toolName)) {
            return { behavior: "allow" }
          }

          return {
            behavior: "deny",
            message: `Tool ${toolName} is not allowed in this read-only live check.`,
          }
        },
        permissionMode: "default",
        persistSession: false,
        settingSources: [],
        stderr: (line: string) => pushBounded(summary.stderrLines, line),
        systemPrompt: {
          append:
            "Inspect the repository directly before answering. Use only the available read-only tools, inspect representative files instead of trying to read everything, and do not rely on prior memory.",
          preset: "claude_code",
          type: "preset",
        },
        tools: allowedTools,
      },
    })

    for await (const message of messageStream) {
      summary.messageCount += 1

      if (message.type === "assistant") {
        for (const block of assistantBlocks(message)) {
          if (block.type === "tool_use") {
            const toolName = block.name ?? "unknown"
            const toolUseId = block.id ?? "unknown"
            const inputPreview = preview(safeStringify(block.input))
            summary.toolUses.push({ inputPreview, toolName, toolUseId })
            trackTool(summary.toolCounts, toolName)
            console.log(`[tool_use] ${toolName} (${toolUseId}) ${inputPreview}`)
            continue
          }

          if (block.type === "text" && typeof block.text === "string") {
            summary.assistantText.push(block.text)
          }
        }
        continue
      }

      if (message.type === "tool_progress") {
        trackTool(summary.toolCounts, message.tool_name)
        console.log(
          `[tool_progress] ${message.tool_name} (${message.tool_use_id}) ${message.elapsed_time_seconds.toFixed(1)}s`,
        )
        continue
      }

      if (message.type === "tool_use_summary") {
        console.log(`[tool_use_summary] ${preview(message.summary)}`)
        continue
      }

      if (message.type === "stream_event") {
        summary.streamEventCount += 1
        continue
      }

      if (message.type === "result") {
        summary.resultMessage = message
        continue
      }

      if (
        message.type === "system"
        && "subtype" in message
        && message.subtype === "api_retry"
      ) {
        console.log(
          `[api_retry] attempt ${message.attempt}/${message.max_retries} after ${message.error}`,
        )
      }
    }
  } catch (error: unknown) {
    const message = error instanceof Error ? error.message : String(error)
    console.error("")
    console.error(`Claude Agent SDK run failed: ${message}`)
    if (summary.stderrLines.length > 0) {
      console.error("")
      console.error("Recent SDK stderr")
      for (const line of summary.stderrLines) {
        console.error(line.trimEnd())
      }
    }
    process.exit(1)
  } finally {
    clearTimeout(timeout)
  }

  const failures = validateSummary(summary)
  printSummary(summary)

  if (failures.length > 0) {
    console.error("")
    console.error("Validation failed")
    for (const failure of failures) {
      console.error(`- ${failure}`)
    }

    if (summary.stderrLines.length > 0) {
      console.error("")
      console.error("Recent SDK stderr")
      for (const line of summary.stderrLines) {
        console.error(line.trimEnd())
      }
    }

    process.exit(1)
  }

  console.log("")
  console.log("Claude Agent SDK live check passed.")
}

await main()
