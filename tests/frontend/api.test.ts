import { afterEach, beforeEach, describe, expect, mock, test } from "bun:test"

import {
  activateProvider,
  createChatCompletion,
  getChatSession,
  getLogLevel,
  listChatSessions,
  listConfigFiles,
  listProviders,
  subscribeToLogs,
  updateLogLevel,
} from "../../frontend/src/api"
import {
  EventSourceMock,
  resetTestEnvironment,
  setupFetchMocks,
  setupTestEnvironment,
} from "./setup"

describe("frontend api helpers", () => {
  let mockFetch: ReturnType<typeof mock> | null = null

  beforeEach(() => {
    setupTestEnvironment()
  })

  afterEach(() => {
    resetTestEnvironment()
    mockFetch?.mockClear()
  })

  test("listProviders fetches provider list", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/api/admin/providers": {
        GET: {
          response: [
            {
              id: "provider-1",
              type: "alibaba",
              name: "Alibaba",
              isActive: true,
              authStatus: "authenticated",
            },
          ],
        },
      },
    })

    const providers = await listProviders()

    expect(providers).toHaveLength(1)
    expect(providers[0]?.id).toBe("provider-1")
  })

  test("activateProvider posts to activate endpoint", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/api/admin/providers/provider-1/activate": {
        POST: {
          response: {
            success: true,
            provider: { id: "provider-1", name: "Alibaba" },
          },
        },
      },
    })

    const result = await activateProvider("provider-1")

    expect(result.success).toBe(true)

    const call = mockFetch.mock.calls.find(([url]) =>
      new URL(String(url), "http://localhost:4141").pathname
      === "/api/admin/providers/provider-1/activate"
    )
    expect(call?.[1]?.method).toBe("POST")
  })

  test("getLogLevel normalizes warning to warn", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/api/admin/settings/log-level": {
        GET: { response: { level: "warning" } },
      },
    })

    const result = await getLogLevel()

    expect(result.level).toBe("warn")
  })

  test("updateLogLevel normalizes numeric response levels", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/api/admin/settings/log-level": {
        PUT: { response: { success: true, level: 4 } },
      },
    })

    const result = await updateLogLevel("debug")

    expect(result.success).toBe(true)
    expect(result.level).toBe("debug")
  })

  test("createChatCompletion converts openai messages to responses input", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/v1/responses": {
        POST: {
          response: {
            id: "resp_123",
            object: "realtime.response",
            model: "gpt-5.4-mini",
            output: [],
            usage: { input_tokens: 1, output_tokens: 2, total_tokens: 3 },
          },
        },
      },
    })

    const result = await createChatCompletion(
      {
        model: "gpt-5.4-mini",
        messages: [
          { role: "system", content: "Be terse." },
          { role: "user", content: "Hello" },
        ],
        max_tokens: 64,
        temperature: 0.2,
      },
      "responses",
    )

    expect("object" in result && result.object).toBe("realtime.response")

    const call = mockFetch.mock.calls.find(([url]) =>
      new URL(String(url), "http://localhost:4141").pathname === "/v1/responses"
    )
    expect(call).toBeDefined()
    const body = JSON.parse(String(call?.[1]?.body))
    expect(body).toEqual({
      model: "gpt-5.4-mini",
      input: [
        { type: "message", role: "system", content: "Be terse." },
        { type: "message", role: "user", content: "Hello" },
      ],
      max_output_tokens: 64,
      stream: false,
      temperature: 0.2,
    })
  })

  test("listChatSessions supports wrapped session payloads", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/api/admin/chat/sessions": {
        GET: {
          response: {
            sessions: [
              {
                session_id: "session-1",
                title: "Test Session",
                model_id: "gpt-4",
                api_shape: "openai",
                created_at: "2026-04-20T00:00:00Z",
                updated_at: "2026-04-20T00:00:00Z",
              },
            ],
          },
        },
      },
    })

    const sessions = await listChatSessions()

    expect(sessions).toHaveLength(1)
    expect(sessions[0]?.session_id).toBe("session-1")
  })

  test("getChatSession normalizes go backend shape", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/api/admin/chat/sessions/session-1": {
        GET: {
          response: {
            id: "session-1",
            title: "Go Session",
            model_id: "gpt-4",
            api_shape: "responses",
            created_at: "2026-04-20T00:00:00Z",
            updated_at: "2026-04-20T00:00:00Z",
            messages: [
              {
                id: "msg-1",
                role: "user",
                content: "Hello",
                created_at: "2026-04-20T00:00:01Z",
              },
            ],
          },
        },
      },
    })

    const session = await getChatSession("session-1")

    expect(session.session.session_id).toBe("session-1")
    expect(session.messages[0]?.message_id).toBe("msg-1")
    expect(session.messages[0]?.content).toBe("Hello")
  })

  test("listConfigFiles returns config metadata", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
      "/api/admin/config": {
        GET: {
          response: {
            configs: [
              {
                name: "config.toml",
                label: "Config",
                description: "Main config",
                language: "toml",
                exists: true,
              },
            ],
          },
        },
      },
    })

    const result = await listConfigFiles()

    expect(result.configs).toHaveLength(1)
    expect(result.configs[0]?.name).toBe("config.toml")
  })

  test("subscribeToLogs creates an EventSource and forwards messages", async () => {
    mockFetch = setupFetchMocks(globalThis, {
      "/api/admin/info": {
        GET: { response: { version: "test", port: 4141 } },
      },
    })

    const lines: Array<string> = []
    const es = await subscribeToLogs((line) => lines.push(line))

    expect(es).toBeInstanceOf(EventSourceMock)
    expect((es as EventSourceMock).url).toContain("/api/admin/logs/stream")

    ;(es as EventSourceMock)._triggerMessage("hello")
    expect(lines).toHaveLength(1)
    expect((lines[0] as unknown as { data: string }).data).toBe("hello")
  })
})
