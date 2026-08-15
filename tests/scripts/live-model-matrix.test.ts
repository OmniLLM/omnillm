import { describe, expect, test } from "bun:test"

import {
  parseManifest,
  readStream,
  replayRequest,
  sanitizeFailure,
} from "../../scripts/test-live-model-matrix"

const copilotGrokRow = {
  id: "copilot-grok",
  provider: "github-copilot",
  model: "grok-4.5",
  shapes: { chat: true, messages: true, responses: true },
  credentials: [{ env: "LIVE_GITHUB_TOKEN" }],
  capabilities: {
    tools: true,
    parallelTools: true,
    largeResults: true,
    longStream: true,
    cancellation: true,
  },
  scenarioOverrides: {
    parallel_tools: { notApplicable: "account-specific behavior" },
  },
  provision: {
    path: "/api/admin/providers/auth-and-create/github-copilot",
    body: {
      method: "token",
      token: { $env: "LIVE_GITHUB_TOKEN" },
    },
    activate: true,
  },
}

const validRow = {
  id: "example",
  provider: "google",
  model: "gemini-test",
  shapes: {
    chat: true,
    messages: { notApplicable: "not supported" },
    responses: true,
  },
  credentials: [{ env: "LIVE_KEY" }],
  capabilities: {
    tools: true,
    parallelTools: { notApplicable: "not supported" },
    largeResults: true,
    longStream: true,
    cancellation: true,
  },
  provision: {
    path: "/api/admin/providers/auth-and-create/google",
    body: { apiKey: { $env: "LIVE_KEY" } },
  },
}

describe("live model matrix manifest", () => {
  test("parses explicit support and not-applicable declarations", () => {
    const manifest = parseManifest({ version: 1, rows: [validRow] })

    expect(manifest.rows).toHaveLength(1)
    expect(manifest.rows[0]?.shapes.messages).toEqual({
      notApplicable: "not supported",
    })
    expect(manifest.rows[0]?.credentials).toEqual([{ env: "LIVE_KEY" }])
  })

  test("accepts GitHub Copilot Grok provisioning by environment reference", () => {
    const manifest = parseManifest({ version: 1, rows: [copilotGrokRow] })

    expect(manifest.rows[0]).toEqual(copilotGrokRow)
  })

  test("rejects duplicate row identifiers", () => {
    expect(() =>
      parseManifest({ version: 1, rows: [validRow, validRow] }),
    ).toThrow("duplicate row id")
  })

  test("accepts isolated token bundle references", () => {
    const row = {
      ...validRow,
      credentials: [
        {
          tokenBundleEnv: "LIVE_BUNDLE",
          target: ".config/omnillm/token.json",
        },
      ],
    }
    const manifest = parseManifest({ version: 1, rows: [row] })

    expect(manifest.rows[0]?.credentials).toEqual(row.credentials)
  })

  test("rejects schema-invalid credential and unknown fields", () => {
    expect(() =>
      parseManifest({
        version: 1,
        rows: [
          {
            ...validRow,
            credentials: [
              {
                env: "LIVE_KEY",
                tokenBundleEnv: "LIVE_BUNDLE",
                target: ".config/omnillm/token.json",
              },
            ],
          },
        ],
      }),
    ).toThrow("exactly one credential reference")
    expect(() =>
      parseManifest({ version: 1, rows: [{ ...validRow, apiKey: "secret" }] }),
    ).toThrow("unknown field")
  })

  test("rejects invalid environment names and excessive budgets", () => {
    expect(() =>
      parseManifest({
        version: 1,
        rows: [{ ...validRow, credentials: [{ env: "lower-case" }] }],
      }),
    ).toThrow("environment variable name")
    expect(() =>
      parseManifest({
        version: 1,
        budgets: { requestMs: 300_001 },
        rows: [validRow],
      }),
    ).toThrow("between 1000ms and 300000ms")
  })
})

describe("live model matrix tool history", () => {
  test("appends repeated chat tool turns to existing history", () => {
    const firstRequest = {
      model: "model",
      messages: [{ role: "user", content: "first" }],
      tool_choice: "required",
    }
    const firstReplay = replayRequest(
      "chat",
      "model",
      {
        payload: {
          tool_calls: [
            {
              id: "call-1",
              type: "function",
              function: { name: "matrix_echo", arguments: "{}" },
            },
          ],
        },
        calls: [{ id: "call-1", name: "matrix_echo" }],
      },
      "cycle-0",
      true,
      firstRequest,
    ) as { messages: Array<unknown> }
    const secondReplay = replayRequest(
      "chat",
      "model",
      {
        payload: {
          tool_calls: [
            {
              id: "call-2",
              type: "function",
              function: { name: "matrix_echo", arguments: "{}" },
            },
          ],
        },
        calls: [{ id: "call-2", name: "matrix_echo" }],
      },
      "cycle-1",
      true,
      firstReplay,
    ) as { messages: Array<Record<string, unknown>> }

    expect(secondReplay.messages).toHaveLength(7)
    expect(
      secondReplay.messages.some(
        (message) => message.tool_call_id === "call-1",
      ),
    ).toBe(true)
    expect(
      secondReplay.messages.some(
        (message) => message.tool_call_id === "call-2",
      ),
    ).toBe(true)
  })
})

describe("live model matrix streams", () => {
  test("counts chunks and SSE data frames", async () => {
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode("data: one\n\n"))
        controller.enqueue(new TextEncoder().encode("data: two\n\n"))
        controller.close()
      },
    })
    const stats = await readStream(new Response(stream))

    expect(stats.chunks).toBe(2)
    expect(stats.dataFrames).toBe(2)
  })
})

describe("live model matrix reporting", () => {
  test("redacts declared secrets and common credential syntax", () => {
    const secret = "synthetic-super-secret"
    const message = sanitizeFailure(
      new Error(`Authorization: Bearer ${secret}; api_key=${secret}`),
      [secret],
    )

    expect(message).not.toContain(secret)
    expect(message).toContain("[REDACTED]")
  })
})
