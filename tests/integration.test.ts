/**
 * Integration tests against a running proxy server at localhost:5000.
 *
 * Run with: bun test tests/integration.test.ts
 *
 * Requires the proxy to be running: bun run omni:start -- --server-port 5000
 */

import { describe, test, expect, beforeAll } from "bun:test"

import {
  formatDangerousTestSkipMessage,
  RUN_DANGEROUS_TESTS,
} from "./test-config"

const BASE = "http://localhost:5000"
const dangerousDescribe = RUN_DANGEROUS_TESTS ? describe : describe.skip

if (!RUN_DANGEROUS_TESTS) {
  console.warn(formatDangerousTestSkipMessage("tests/integration.test.ts"))
}

// Test-specific logging that provides clear output during test runs
const testLog = {
  info: (message: string) => console.log(`[TEST][TEST] ${message}`),
  warn: (message: string) => console.warn(`[TEST] ⚠️  ${message}`),
  error: (message: string) => console.error(`[TEST] ❌ ${message}`),
  success: (message: string) => console.log(`[TEST][TEST] ✅ ${message}`),
}

async function post(
  path: string,
  body: unknown,
  extraHeaders?: Record<string, string>,
) {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...extraHeaders,
    },
    body: JSON.stringify(body),
  })
  const json = await res.json()
  if (!res.ok) {
    testLog.error(
      `Request failed: ${res.status}`,
      JSON.stringify(json, null, 2),
    )
  }
  return { status: res.status, json }
}

async function get(path: string, extraHeaders?: Record<string, string>) {
  const res = await fetch(`${BASE}${path}`, {
    method: "GET",
    headers: {
      "Content-Type": "application/json",
      ...extraHeaders,
    },
  })
  const json = await res.json()
  if (!res.ok) {
    testLog.error(
      `GET request failed: ${res.status}`,
      JSON.stringify(json, null, 2),
    )
  }
  return { status: res.status, json }
}

// Enhanced tool definitions for realistic testing
const WEATHER_TOOL = {
  type: "function",
  name: "get_weather",
  description: "Get current weather information for a specific city",
  parameters: {
    type: "object",
    properties: {
      location: {
        type: "string",
        description: "The city name, e.g. 'Shanghai' or 'New York'",
      },
      units: {
        type: "string",
        enum: ["celsius", "fahrenheit"],
        description: "Temperature units",
      },
    },
    required: ["location"],
  },
}

const CODE_GENERATOR_TOOL = {
  type: "function",
  name: "generate_typescript",
  description: "Generate TypeScript code based on requirements",
  parameters: {
    type: "object",
    properties: {
      requirement: {
        type: "string",
        description: "Description of what the TypeScript code should do",
      },
      includeTypes: {
        type: "boolean",
        description: "Whether to include TypeScript type definitions",
      },
    },
    required: ["requirement"],
  },
}

const TIME_TOOL = {
  type: "function",
  name: "get_current_time",
  description: "Get the current time",
  parameters: {
    type: "object",
    properties: {
      format: {
        type: "string",
        enum: ["12hour", "24hour", "iso"],
        description: "Time format to return",
      },
    },
  },
}

// OpenAI format tools for chat completions
const OPENAI_WEATHER_TOOL = {
  type: "function",
  function: WEATHER_TOOL,
}

const OPENAI_CODE_TOOL = {
  type: "function",
  function: CODE_GENERATOR_TOOL,
}

const OPENAI_TIME_TOOL = {
  type: "function",
  function: TIME_TOOL,
}

// Anthropic format tools for messages endpoint
const ANTHROPIC_WEATHER_TOOL = {
  name: "get_weather",
  description: "Get current weather information for a specific city",
  input_schema: {
    type: "object",
    properties: {
      location: {
        type: "string",
        description: "The city name, e.g. 'Shanghai' or 'New York'",
      },
      units: {
        type: "string",
        enum: ["celsius", "fahrenheit"],
        description: "Temperature units",
      },
    },
    required: ["location"],
  },
}

const ANTHROPIC_CODE_TOOL = {
  name: "generate_typescript",
  description: "Generate TypeScript code based on requirements",
  input_schema: {
    type: "object",
    properties: {
      requirement: {
        type: "string",
        description: "Description of what the TypeScript code should do",
      },
      includeTypes: {
        type: "boolean",
        description: "Whether to include TypeScript type definitions",
      },
    },
    required: ["requirement"],
  },
}

const ANTHROPIC_TIME_TOOL = {
  name: "get_current_time",
  description: "Get the current time",
  input_schema: {
    type: "object",
    properties: {
      format: {
        type: "string",
        enum: ["12hour", "24hour", "iso"],
        description: "Time format to return",
      },
    },
  },
}

// Helper functions for provider management
async function getProviders() {
  const { status, json } = await get("/api/admin/providers")
  if (status !== 200) {
    throw new Error(`Failed to get providers: ${JSON.stringify(json)}`)
  }
  return json as Array<{
    id: string
    type: string
    name: string
    isActive: boolean
    authStatus: string
    enabledModelCount: number
    totalModelCount: number
  }>
}

async function activateProvider(providerId: string) {
  const { status, json } = await post(
    `/api/admin/providers/${providerId}/activate`,
    {},
  )
  testLog.info(
    `Activating provider ${providerId}: ${status} - ${JSON.stringify(json)}`,
  )
  return { status, json }
}

async function deactivateProvider(providerId: string) {
  const { status, json } = await post(
    `/api/admin/providers/${providerId}/deactivate`,
    {},
  )
  testLog.info(
    `Deactivating provider ${providerId}: ${status} - ${JSON.stringify(json)}`,
  )
  return { status, json }
}

let initialProviderState: Array<{ id: string; isActive: boolean }> = []

beforeAll(async () => {
  if (!RUN_DANGEROUS_TESTS) {
    return
  }

  try {
    const res = await fetch(`${BASE}/healthz`).catch(() => null)
    if (!res || !res.ok) {
      throw new Error("Proxy not responding — is it running on port 5005?")
    }
  } catch {
    // healthz may not exist; do a lightweight probe instead
    const probe = await fetch(`${BASE}/v1/models`).catch(() => null)
    if (!probe) {
      throw new Error("Proxy not reachable at localhost:5005 — start it first")
    }
  }

  // Store initial provider state
  try {
    const providers = await getProviders()
    initialProviderState = providers.map((p) => ({
      id: p.id,
      isActive: p.isActive,
    }))
    testLog.info("Initial provider state:", initialProviderState)
  } catch (err) {
    testLog.warn("Failed to get initial provider state:", err)
  }
})

// Helper to ensure a provider is active before tests
async function ensureProviderActive(providerId: string) {
  await activateProvider(providerId)
}

// ---------------------------------------------------------------------------
// GitHub Copilot — claude-haiku-4.5 and claude-sonnet-4.6
// ---------------------------------------------------------------------------

dangerousDescribe("GitHub Copilot - claude-haiku-4.5", () => {
  test("basic message", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-haiku-4.5",
        max_tokens: 20,
        messages: [{ role: "user", content: "Reply with just the word: pong" }],
      },
      { "anthropic-version": "2023-06-01" },
    )
    testLog.success(`Basic message test (Haiku) - Status: ${status}`)
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  })

  test("weather tool call for Shanghai", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-haiku-4.5",
        max_tokens: 200,
        tools: [ANTHROPIC_WEATHER_TOOL],
        messages: [
          {
            role: "user",
            content:
              "What is the weather today in Shanghai? Use the weather tool to check.",
          },
        ],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]🌤️ Weather tool test (Haiku) - Status: ${status}`)

    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")

    // Tool use should work for haiku
    if (response.stop_reason === "tool_use") {
      const toolCall = response.content.find((item) => item.type === "tool_use")
      expect(toolCall).toBeDefined()
      expect(toolCall.name).toBe("get_weather")
      expect(toolCall.input.location).toEqual("Shanghai")
      console.log(`[TEST]✅ Weather tool called successfully for Shanghai by Haiku`)
    } else {
      console.warn("⚠️ Tool was not used, stop_reason:", response.stop_reason)
    }
  })

  test("TypeScript code generation tool", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-haiku-4.5",
        max_tokens: 300,
        tools: [ANTHROPIC_CODE_TOOL, ANTHROPIC_TIME_TOOL],
        messages: [
          {
            role: "user",
            content:
              "Write a simple TypeScript function to show me the current time. Use the appropriate tools.",
          },
        ],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]💻 TypeScript tool test (Haiku) - Status: ${status}`)

    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")

    if (response.stop_reason === "tool_use") {
      const toolCalls = response.content.filter(
        (item) => item.type === "tool_use",
      )
      expect(toolCalls.length).toBeGreaterThan(0)

      // Should use either code generation or time tool (or both)
      const toolNames = toolCalls.map((call) => call.name)
      expect(
        toolNames.some((name) =>
          ["generate_typescript", "get_current_time"].includes(name),
        ),
      ).toBe(true)
      console.log(`[TEST]✅ Tools used by Haiku:`, toolNames)
    } else {
      console.warn(
        "⚠️ No tools used for TypeScript generation, stop_reason:",
        response.stop_reason,
      )
    }
  })

  test("via chat completions endpoint", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post("/v1/chat/completions", {
      model: "claude-haiku-4.5",
      messages: [{ role: "user", content: "Reply with just the word: ping" }],
      max_tokens: 10,
    })
    console.log(`[TEST]💬 Chat completions test (Haiku) - Status: ${status}`)
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (json.choices as Array<{ message: { role: string } }>)[0]
    expect(choice.message.role).toBe("assistant")
  })
})

dangerousDescribe("GitHub Copilot - claude-sonnet-4.6", () => {
  test("basic message", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-sonnet-4.6",
        max_tokens: 20,
        messages: [{ role: "user", content: "Reply with just the word: pong" }],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]✅ Basic message test (Sonnet) - Status: ${status}`)
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  })

  test("weather tool call for Shanghai with multiple tools", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-sonnet-4.6",
        max_tokens: 300,
        tools: [ANTHROPIC_WEATHER_TOOL, ANTHROPIC_TIME_TOOL],
        messages: [
          {
            role: "user",
            content:
              "What is the weather today in Shanghai? Also tell me what time it is. Use the appropriate tools.",
          },
        ],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]🌤️⏰ Multi-tool test (Sonnet) - Status: ${status}`)

    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")

    // Tool use should work for sonnet
    if (response.stop_reason === "tool_use") {
      const toolCalls = response.content.filter(
        (item) => item.type === "tool_use",
      )
      expect(toolCalls.length).toBeGreaterThan(0)

      // Should use weather tool and possibly time tool
      const weatherCall = toolCalls.find((call) => call.name === "get_weather")
      expect(weatherCall).toBeDefined()
      if (weatherCall) {
        expect(weatherCall.input.location).toEqual("Shanghai")
      }

      const toolNames = toolCalls.map((call) => call.name)
      console.log(`[TEST]✅ Tools used by Sonnet:`, toolNames)
    } else {
      console.warn("⚠️ Tools were not used, stop_reason:", response.stop_reason)
    }
  })

  test("complex TypeScript generation", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-sonnet-4.6",
        max_tokens: 400,
        tools: [ANTHROPIC_CODE_TOOL],
        messages: [
          {
            role: "user",
            content:
              "Generate a TypeScript interface for a user profile with name, email, and optional avatar URL. Use the code generation tool.",
          },
        ],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]💻 Complex TypeScript test (Sonnet) - Status: ${status}`)

    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")

    if (response.stop_reason === "tool_use") {
      const toolCall = response.content.find((item) => item.type === "tool_use")
      expect(toolCall).toBeDefined()
      expect(toolCall.name).toBe("generate_typescript")
      expect(toolCall.input.requirement).toContain("interface")
      console.log(`[TEST]✅ TypeScript generation tool used by Sonnet`)
    }
  })

  test("via chat completions endpoint", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")
    const { status, json } = await post("/v1/chat/completions", {
      model: "claude-sonnet-4.6",
      messages: [{ role: "user", content: "Reply with just the word: ping" }],
      max_tokens: 10,
    })
    console.log(`[TEST]💬 Chat completions test (Sonnet) - Status: ${status}`)
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (json.choices as Array<{ message: { role: string } }>)[0]
    expect(choice.message.role).toBe("assistant")
  })
})

// ---------------------------------------------------------------------------
// Azure OpenAI — test available models with enhanced tool usage
// ---------------------------------------------------------------------------

dangerousDescribe("Azure OpenAI - gpt-5.4-mini", () => {
  test("basic chat completion", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.4-mini",
      messages: [{ role: "user", content: "Reply with just the word: pong" }],
      max_tokens: 10,
    })
    console.log(`[TEST]✅ Basic chat test (GPT-5.4-mini) - Status: ${status}`)
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (json.choices as Array<{ message: { role: string } }>)[0]
    expect(choice.message.role).toBe("assistant")
  })

  test("weather tool call for Shanghai", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.4-mini",
      messages: [
        {
          role: "user",
          content:
            "What is the weather today in Shanghai? Use the weather tool to check.",
        },
      ],
      tools: [OPENAI_WEATHER_TOOL],
      max_tokens: 200,
    })
    console.log(`[TEST]🌤️ GPT-5.4-mini weather test - Status: ${status}`)

    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (
      json.choices as Array<{ message: { tool_calls?: Array<any> } }>
    )[0]

    // Check if tool was used
    if (choice.message.tool_calls && choice.message.tool_calls.length > 0) {
      expect(choice.message.tool_calls[0].function.name).toBe("get_weather")
      const args = JSON.parse(choice.message.tool_calls[0].function.arguments)
      expect(args.location).toEqual("Shanghai")
      console.log(
        `✅ Weather tool called successfully for Shanghai by GPT-5.4-mini`,
      )
    } else {
      console.warn("⚠️ Weather tool was not used by GPT-5.4-mini")
    }
  })

  test("TypeScript code generation tool", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.4-mini",
      messages: [
        {
          role: "user",
          content:
            "Write a simple TypeScript function to show me the current time. Use the code generation tool.",
        },
      ],
      tools: [OPENAI_CODE_TOOL, OPENAI_TIME_TOOL],
      max_tokens: 300,
    })
    console.log(`[TEST]💻 GPT-5.4-mini TypeScript test - Status: ${status}`)

    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (
      json.choices as Array<{ message: { tool_calls?: Array<any> } }>
    )[0]

    if (choice.message.tool_calls && choice.message.tool_calls.length > 0) {
      const toolNames = choice.message.tool_calls.map(
        (call) => call.function.name,
      )
      expect(
        toolNames.some((name) =>
          ["generate_typescript", "get_current_time"].includes(name),
        ),
      ).toBe(true)
      console.log(`[TEST]✅ Tools used by GPT-5.4-mini:`, toolNames)
    } else {
      console.warn("⚠️ No tools used for TypeScript generation by GPT-5.4-mini")
    }
  })
})

// ---------------------------------------------------------------------------
// Provider Management Tests
// ---------------------------------------------------------------------------

dangerousDescribe("Provider Management", () => {
  test("get provider list", async () => {
    const { status, json } = await get("/api/admin/providers")
    console.log(`[TEST]📋 Provider list - Status: ${status}`)
    console.log(
      `🔧 Providers:`,
      json.map(
        (p: any) =>
          `${p.id} (${p.type}) - ${p.isActive ? "active" : "inactive"} - ${p.authStatus} - ${p.enabledModelCount}/${p.totalModelCount} models`,
      ),
    )

    expect(status).toBe(200)
    expect(Array.isArray(json)).toBe(true)

    for (const provider of json) {
      expect(provider).toHaveProperty("id")
      expect(provider).toHaveProperty("type")
      expect(provider).toHaveProperty("isActive")
      expect(provider).toHaveProperty("authStatus")
    }
  })

  test("provider activation and deactivation", async () => {
    const providers = await getProviders()
    const authenticatedProviders = providers.filter(
      (p) => p.authStatus === "authenticated",
    )

    if (authenticatedProviders.length === 0) {
      console.warn(
        "⚠️ No authenticated providers found, skipping activation test",
      )
      return
    }

    const testProvider = authenticatedProviders[0]
    const initialState = testProvider.isActive
    console.log(
      `🧪 Testing provider ${testProvider.id} - initial state: ${initialState ? "active" : "inactive"}`,
    )

    try {
      // Test deactivation if currently active
      if (initialState) {
        const deactivateResult = await deactivateProvider(testProvider.id)
        expect(deactivateResult.status).toBe(200)

        // Verify deactivation
        const providersAfterDeactivate = await getProviders()
        const deactivatedProvider = providersAfterDeactivate.find(
          (p) => p.id === testProvider.id,
        )
        expect(deactivatedProvider?.isActive).toBe(false)
      }

      // Test activation
      const activateResult = await activateProvider(testProvider.id)
      expect(activateResult.status).toBe(200)

      // Verify activation
      const providersAfterActivate = await getProviders()
      const activatedProvider = providersAfterActivate.find(
        (p) => p.id === testProvider.id,
      )
      expect(activatedProvider?.isActive).toBe(true)
    } finally {
      // Restore initial state
      await (initialState ?
        activateProvider(testProvider.id)
      : deactivateProvider(testProvider.id))
    }
  })

  test("tool calls work after provider switch", async () => {
    const providers = await getProviders()
    const authenticatedProviders = providers.filter(
      (p) => p.authStatus === "authenticated" && p.totalModelCount > 0,
    )

    if (authenticatedProviders.length < 2) {
      console.warn(
        "⚠️ Need at least 2 authenticated providers to test switching, skipping",
      )
      return
    }

    console.log(`[TEST]\n🔄 Testing provider switching with tool calls`)

    // Test with GitHub Copilot and Azure OpenAI specifically
    const githubProvider = authenticatedProviders.find(
      (p) => p.type === "github-copilot",
    )
    const azureProvider = authenticatedProviders.find(
      (p) => p.type === "azure-openai",
    )

    if (githubProvider) {
      console.log(
        `\n🧪 Testing tool calls with GitHub Copilot: ${githubProvider.id}`,
      )

      // Activate only GitHub Copilot
      for (const p of providers) {
        await (p.id === githubProvider.id ?
          activateProvider(p.id)
        : deactivateProvider(p.id))
      }

      // Test Anthropic Messages endpoint with tool
      const { status, json } = await post(
        "/v1/messages",
        {
          model: "claude-haiku-4.5",
          max_tokens: 200,
          tools: [ANTHROPIC_WEATHER_TOOL],
          messages: [
            {
              role: "user",
              content: "What is the weather in Shanghai? Use the tool.",
            },
          ],
        },
        { "anthropic-version": "2023-06-01" },
      )
      console.log(`[TEST]  📥 Messages API result: ${status}`)

      if (status === 200) {
        expect(json.type).toBe("message")
        if (json.stop_reason === "tool_use") {
          console.log(`[TEST]  ✅ Tool successfully used via Messages API`)
        } else {
          console.log(`[TEST]  ⚠️ Tool not used, stop_reason: ${json.stop_reason}`)
        }
      }
    }

    if (azureProvider) {
      console.log(
        `\n🧪 Testing tool calls with Azure OpenAI: ${azureProvider.id}`,
      )

      // Activate only Azure OpenAI
      for (const p of providers) {
        await (p.id === azureProvider.id ?
          activateProvider(p.id)
        : deactivateProvider(p.id))
      }

      // Test Chat Completions endpoint with tool
      const { status, json } = await post("/v1/chat/completions", {
        model: "gpt-5.4-mini",
        messages: [
          {
            role: "user",
            content: "What is the weather in Shanghai? Use the tool.",
          },
        ],
        tools: [OPENAI_WEATHER_TOOL],
        max_tokens: 200,
      })
      console.log(`[TEST]  💬 Chat Completions API result: ${status}`)

      if (status === 200) {
        expect(json.choices).toBeDefined()
        const choice = json.choices[0]
        if (choice.message.tool_calls && choice.message.tool_calls.length > 0) {
          console.log(`[TEST]  ✅ Tool successfully used via Chat Completions API`)
        } else {
          console.log(`[TEST]  ⚠️ Tool not used`)
        }
      }
    }
  })
})

// ---------------------------------------------------------------------------
// Error Handling and Edge Cases
// ---------------------------------------------------------------------------

dangerousDescribe("Error Handling", () => {
  test("invalid model request", async () => {
    const { status, json } = await post("/v1/chat/completions", {
      model: "nonexistent-model-12345",
      messages: [{ role: "user", content: "Hello" }],
      max_tokens: 10,
    })
    console.log(`[TEST]❌ Invalid model test - Status: ${status}`)

    expect(status).toBe(400)
    expect(json.error).toBeDefined()
    expect(json.error.message).toContain("not found")
  })

  test("malformed tool call handling", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.4-mini",
      messages: [{ role: "user", content: "Hello" }],
      tools: [
        OPENAI_WEATHER_TOOL,
        {
          type: "function",
          function: {
            name: "",
            description: "This tool has no name and should be filtered out",
          },
        },
      ],
      max_tokens: 10,
    })
    console.log(`[TEST]🛠️ Malformed tool test - Status: ${status}`)

    // Should either work (filtering out malformed tool) or return proper validation error
    expect([200, 400]).toContain(status)
    if (status === 200) {
      expect(json.choices).toBeDefined()
      console.log(`[TEST]✅ Malformed tool was properly filtered`)
    } else if (status === 400) {
      expect(json.error).toBeDefined()
      console.log(`[TEST]✅ Proper validation error for malformed tool`)
    }
  })

  test("no active providers error", async () => {
    // Deactivate all providers
    const providers = await getProviders()
    const activeProviders = providers.filter((p) => p.isActive)

    try {
      console.log(`[TEST]🔄 Deactivating all providers to test error handling`)
      for (const provider of activeProviders) {
        await deactivateProvider(provider.id)
      }

      // Try to make a request
      const { status, json } = await post("/v1/chat/completions", {
        model: "gpt-5.4-mini",
        messages: [{ role: "user", content: "Hello" }],
        max_tokens: 10,
      })
      console.log(`[TEST]❌ No active providers test - Status: ${status}`)

      expect(status).toBe(400)
      expect(json.error).toBeDefined()
      expect(json.error.message).toContain("No active providers")
    } finally {
      // Restore active providers
      console.log(`[TEST]🔄 Restoring active providers`)
      for (const provider of activeProviders) {
        await activateProvider(provider.id)
      }
    }
  })
})

// ---------------------------------------------------------------------------
// Azure OpenAI GPT-5.4 — Test responses API and longer conversations
// ---------------------------------------------------------------------------

dangerousDescribe("Azure OpenAI - gpt-5.4 (Responses API)", () => {
  test("basic message via responses API", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.4",
        max_tokens: 50,
        messages: [{ role: "user", content: "Reply with just the word: pong" }],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]✅ Basic responses API test (GPT-5.4) - Status: ${status}`)
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  })

  test("longer conversation (10+ messages) - tests fix for index 8+ content issue", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")

    // Create a longer conversation that would trigger the original bug
    const messages = [
      {
        role: "user",
        content: [{ type: "text", text: "Hi, I'd like to ask about math." }],
      },
      {
        role: "assistant",
        content: [
          {
            type: "text",
            text: "Sure! I'd be happy to help with math questions.",
          },
        ],
      },
      { role: "user", content: [{ type: "text", text: "What's 2 + 2?" }] },
      {
        role: "assistant",
        content: [{ type: "text", text: "2 + 2 equals 4." }],
      },
      { role: "user", content: [{ type: "text", text: "And 5 * 6?" }] },
      {
        role: "assistant",
        content: [{ type: "text", text: "5 * 6 equals 30." }],
      },
      { role: "user", content: [{ type: "text", text: "What about 15 / 3?" }] },
      {
        role: "assistant",
        content: [{ type: "text", text: "15 / 3 equals 5." }],
      },
      {
        role: "user",
        content: [
          {
            type: "text",
            text: "This is message at index 8 that used to cause null content error.",
          },
        ],
      },
      {
        role: "assistant",
        content: [
          {
            type: "text",
            text: "I understand. This conversation has progressed well.",
          },
        ],
      },
      {
        role: "user",
        content: [
          {
            type: "text",
            text: "Final question at index 10: What's the square root of 16?",
          },
        ],
      },
    ]

    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.4",
        max_tokens: 100,
        messages,
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(
      `📚 Long conversation test (GPT-5.4, 11 messages) - Status: ${status}`,
    )
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  })

  test("streaming response with longer conversation", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")

    const messages = Array.from({ length: 10 }, (_, i) => [
      {
        role: "user",
        content: [
          { type: "text", text: `Question ${i + 1}: What's ${i + 1} * 2?` },
        ],
      },
      {
        role: "assistant",
        content: [{ type: "text", text: `Answer ${i + 1}: ${(i + 1) * 2}` }],
      },
    ]).flat()

    // Add final user message
    messages.push({
      role: "user",
      content: [
        {
          type: "text",
          text: "Thank you for all the math help! This is a long conversation.",
        },
      ],
    })

    const response = await fetch(`${BASE}/v1/messages`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "anthropic-version": "2023-06-01",
      },
      body: JSON.stringify({
        model: "gpt-5.4",
        max_tokens: 100,
        messages,
        stream: true,
      }),
    })

    console.log(
      `🌊 Streaming long conversation test (GPT-5.4, ${messages.length} messages) - Status: ${response.status}`,
    )
    expect(response.status).toBe(200)

    // Read the first few events to verify it's working
    const reader = response.body?.getReader()
    if (reader) {
      const decoder = new TextDecoder()
      let eventCount = 0
      let hasMessageStart = false

      while (eventCount < 5) {
        const { done, value } = await reader.read()
        if (done) break

        const text = decoder.decode(value)
        if (text.includes('"type":"message_start"')) {
          hasMessageStart = true
        }
        eventCount++
      }

      expect(hasMessageStart).toBe(true)
      reader.releaseLock()
      console.log(
        `✅ Streaming response started successfully for long conversation`,
      )
    }
  })

  test("tool use with longer conversation context", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")

    // Build up a longer conversation with tool context
    const messages = [
      {
        role: "user",
        content: [
          { type: "text", text: "I'm planning a trip and need weather info." },
        ],
      },
      {
        role: "assistant",
        content: [
          {
            type: "text",
            text: "I can help you get weather information using my weather tool!",
          },
        ],
      },
      {
        role: "user",
        content: [{ type: "text", text: "First, what about Shanghai?" }],
      },
      {
        role: "assistant",
        content: [
          { type: "text", text: "Let me check Shanghai's weather for you." },
        ],
      },
      {
        role: "user",
        content: [{ type: "text", text: "Also interested in Beijing." }],
      },
      {
        role: "assistant",
        content: [
          { type: "text", text: "Beijing is another great city to check!" },
        ],
      },
      {
        role: "user",
        content: [{ type: "text", text: "And what about Tokyo?" }],
      },
      {
        role: "assistant",
        content: [{ type: "text", text: "Tokyo weather would be useful too!" }],
      },
      {
        role: "user",
        content: [
          {
            type: "text",
            text: "Actually, let's start with Shanghai. Please check the weather there using your tool.",
          },
        ],
      },
    ]

    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.4",
        max_tokens: 200,
        tools: [ANTHROPIC_WEATHER_TOOL],
        messages,
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(
      `🛠️ Long conversation tool use test (GPT-5.4, 9 messages) - Status: ${status}`,
    )

    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")

    if (response.stop_reason === "tool_use") {
      const toolCall = response.content.find((item) => item.type === "tool_use")
      expect(toolCall).toBeDefined()
      expect(toolCall.name).toBe("get_weather")
      expect(toolCall.input.location).toEqual("Shanghai")
      console.log(
        `✅ Weather tool called successfully in long conversation context`,
      )
    } else {
      console.warn(
        "⚠️ Tool was not used in long conversation, stop_reason:",
        response.stop_reason,
      )
    }
  })

  test("mixed content types in long conversation", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")

    // Test mix of string and array content formats
    const messages = [
      { role: "user", content: "Start of conversation" }, // String format (legacy)
      {
        role: "assistant",
        content: [{ type: "text", text: "I understand, let's continue." }],
      }, // Array format
      { role: "user", content: [{ type: "text", text: "Question 2" }] },
      { role: "assistant", content: "Answer 2" }, // String format
      { role: "user", content: [{ type: "text", text: "Question 3" }] },
      { role: "assistant", content: [{ type: "text", text: "Answer 3" }] },
      { role: "user", content: "Question 4" }, // String format
      { role: "assistant", content: [{ type: "text", text: "Answer 4" }] },
      {
        role: "user",
        content: [
          { type: "text", text: "Final question with mixed content formats" },
        ],
      },
    ]

    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.4",
        max_tokens: 100,
        messages,
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(
      `🔀 Mixed content format test (GPT-5.4, 9 messages) - Status: ${status}`,
    )
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  })

  test("conversation at exactly index 8 (original error case)", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")

    // Create exactly 9 messages so the last one is at index 8 (0-indexed)
    const messages = [
      { role: "user", content: [{ type: "text", text: "Message 0" }] },
      { role: "assistant", content: [{ type: "text", text: "Response 0" }] },
      { role: "user", content: [{ type: "text", text: "Message 1" }] },
      { role: "assistant", content: [{ type: "text", text: "Response 1" }] },
      { role: "user", content: [{ type: "text", text: "Message 2" }] },
      { role: "assistant", content: [{ type: "text", text: "Response 2" }] },
      { role: "user", content: [{ type: "text", text: "Message 3" }] },
      { role: "assistant", content: [{ type: "text", text: "Response 3" }] },
      {
        role: "user",
        content: [
          {
            type: "text",
            text: "This is message at index 8 that previously caused 'input[8].content[0].text' null error",
          },
        ],
      },
    ]

    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.4",
        max_tokens: 100,
        messages,
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(
      `🎯 Index 8 test (GPT-5.4, message at exact error index) - Status: ${status}`,
    )
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  })
})

// ---------------------------------------------------------------------------
// Enhanced Tool Use Tests
// ---------------------------------------------------------------------------

dangerousDescribe("Enhanced Tool Use Cases", () => {
  test("multiple tool calls in sequence with conversation context", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")

    const messages = [
      {
        role: "user",
        content:
          "I need to generate some TypeScript code and then check the time.",
      },
      {
        role: "assistant",
        content:
          "I can help you with both tasks! Let me generate TypeScript code first and then check the current time.",
      },
    ]

    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-sonnet-4.6",
        max_tokens: 500,
        tools: [ANTHROPIC_CODE_TOOL, ANTHROPIC_TIME_TOOL],
        messages: [
          ...messages,
          {
            role: "user",
            content:
              "Generate a TypeScript function for user authentication, and then tell me what time it is. Use both tools please.",
          },
        ],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]🔄 Sequential tool calls test (Sonnet) - Status: ${status}`)

    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")

    if (response.stop_reason === "tool_use") {
      const toolCalls = response.content.filter(
        (item) => item.type === "tool_use",
      )
      const toolNames = toolCalls.map((call) => call.name)
      console.log(`[TEST]🛠️ Tools used:`, toolNames)

      // Should ideally use both tools, but at least one
      expect(toolCalls.length).toBeGreaterThan(0)
      expect(
        toolNames.some((name) =>
          ["generate_typescript", "get_current_time"].includes(name),
        ),
      ).toBe(true)
    } else {
      console.warn(
        "⚠️ No tools used in sequential test, stop_reason:",
        response.stop_reason,
      )
    }
  })

  test("tool use with conversation history and context switching", async () => {
    await ensureProviderActive("github-copilot-jzhu_abk")

    // Build up conversation context across multiple tool usage scenarios
    const conversationHistory = [
      { role: "user", content: "Hi, I'm working on a web application." },
      {
        role: "assistant",
        content: "Great! I can help with web development tasks.",
      },
      {
        role: "user",
        content:
          "I need to check the weather in different cities for my travel app.",
      },
      {
        role: "assistant",
        content: "I can help you get weather information for your travel app.",
      },
      { role: "user", content: "First, let's check Shanghai's weather." },
    ]

    const { status, json } = await post(
      "/v1/messages",
      {
        model: "claude-haiku-4.5",
        max_tokens: 300,
        tools: [ANTHROPIC_WEATHER_TOOL, ANTHROPIC_CODE_TOOL],
        messages: [
          ...conversationHistory,
          {
            role: "user",
            content:
              "Please check the weather in Shanghai using your weather tool, and then suggest some TypeScript code structure for handling weather data.",
          },
        ],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]🌍 Contextual tool use test (Haiku) - Status: ${status}`)

    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")

    if (response.stop_reason === "tool_use") {
      const toolCalls = response.content.filter(
        (item) => item.type === "tool_use",
      )
      const weatherCall = toolCalls.find((call) => call.name === "get_weather")

      if (weatherCall) {
        expect(weatherCall.input.location).toEqual("Shanghai")
        console.log(`[TEST]✅ Weather tool used correctly in context`)
      }

      const toolNames = toolCalls.map((call) => call.name)
      console.log(`[TEST]🛠️ Tools used in context:`, toolNames)
    }
  })

  test("tool use resilience with malformed requests", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")

    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.4-mini",
      messages: [{ role: "user", content: "Check weather in Shanghai" }],
      tools: [
        OPENAI_WEATHER_TOOL,
        // Add a valid tool after invalid ones
        {
          type: "function",
          function: {
            name: "invalid_tool_empty_name",
            description: "This should be filtered out",
            parameters: { type: "object" },
          },
        },
      ],
      max_tokens: 200,
    })
    console.log(`[TEST]🛡️ Tool resilience test (GPT-5.4-mini) - Status: ${status}`)

    // Should work despite malformed tool (filtering it out)
    if (status === 200) {
      expect(json.choices).toBeDefined()
      const choice = json.choices[0]
      if (choice.message.tool_calls && choice.message.tool_calls.length > 0) {
        // Should only use valid tools
        const toolNames = choice.message.tool_calls.map(
          (call) => call.function.name,
        )
        expect(toolNames).not.toContain("invalid_tool_empty_name")
        expect(toolNames).toContain("get_weather")
        console.log(`[TEST]✅ Invalid tools filtered correctly, used:`, toolNames)
      }
    } else {
      // Or should return proper validation error
      expect(json.error).toBeDefined()
      console.log(`[TEST]✅ Proper validation error for malformed tools`)
    }
  })
})

// ---------------------------------------------------------------------------
// Azure OpenAI gpt-5.4-pro — Responses API premium model
// ---------------------------------------------------------------------------

dangerousDescribe("Azure OpenAI - gpt-5.4-pro", () => {
  test("basic message via responses API", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.4-pro",
        max_tokens: 50,
        messages: [{ role: "user", content: "Reply with just the word: pong" }],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]✅ Basic message test (GPT-5.4-pro) - Status: ${status}`)
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  }, 15000)

  test("via chat completions endpoint", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.4-pro",
      messages: [{ role: "user", content: "Reply with just the word: ping" }],
      max_tokens: 10,
    })
    console.log(
      `[TEST]💬 Chat completions test (GPT-5.4-pro) - Status: ${status}`,
    )
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (json.choices as Array<{ message: { role: string } }>)[0]
    expect(choice.message.role).toBe("assistant")
  }, 15000)

  test("weather tool call for Shanghai", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.4-pro",
        max_tokens: 200,
        tools: [ANTHROPIC_WEATHER_TOOL],
        messages: [
          {
            role: "user",
            content:
              "What is the weather today in Shanghai? Use the weather tool to check.",
          },
        ],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(`[TEST]🌤️ Weather tool test (GPT-5.4-pro) - Status: ${status}`)
    expect(status).toBe(200)
    const response = json as {
      type: string
      stop_reason: string
      content: Array<any>
    }
    expect(response.type).toBe("message")
    if (response.stop_reason === "tool_use") {
      const toolCall = response.content.find((item) => item.type === "tool_use")
      expect(toolCall).toBeDefined()
      expect(toolCall.name).toBe("get_weather")
      expect(toolCall.input.location).toEqual("Shanghai")
      console.log(
        `[TEST]✅ Weather tool called successfully for Shanghai by GPT-5.4-pro`,
      )
    } else {
      console.warn(
        "⚠️ Tool was not used, stop_reason:",
        response.stop_reason,
      )
    }
  }, 60000)
})

// ---------------------------------------------------------------------------
// Azure OpenAI gpt-5.3-codex — Responses API code-generation model
// ---------------------------------------------------------------------------

dangerousDescribe("Azure OpenAI - gpt-5.3-codex", () => {
  test("basic message", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.3-codex",
        max_tokens: 50,
        messages: [{ role: "user", content: "Reply with just the word: pong" }],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(
      `[TEST]✅ Basic message test (GPT-5.3-codex) - Status: ${status}`,
    )
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  })

  test("via chat completions endpoint", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.3-codex",
      messages: [{ role: "user", content: "Reply with just the word: ping" }],
      max_tokens: 10,
    })
    console.log(
      `[TEST]💬 Chat completions test (GPT-5.3-codex) - Status: ${status}`,
    )
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (json.choices as Array<{ message: { role: string } }>)[0]
    expect(choice.message.role).toBe("assistant")
  })

  test("TypeScript code generation tool", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.3-codex",
      messages: [
        {
          role: "user",
          content:
            "Write a simple TypeScript function to add two numbers. Use the code generation tool.",
        },
      ],
      tools: [OPENAI_CODE_TOOL],
      max_tokens: 300,
    })
    console.log(
      `[TEST]💻 TypeScript code generation test (GPT-5.3-codex) - Status: ${status}`,
    )
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (
      json.choices as Array<{ message: { tool_calls?: Array<any> } }>
    )[0]
    if (choice.message.tool_calls && choice.message.tool_calls.length > 0) {
      const toolNames = choice.message.tool_calls.map(
        (call) => call.function.name,
      )
      expect(toolNames).toContain("generate_typescript")
      console.log(
        `[TEST]✅ Code generation tool used by GPT-5.3-codex:`,
        toolNames,
      )
    } else {
      console.warn(
        "⚠️ No tools used for TypeScript generation by GPT-5.3-codex",
      )
    }
  })
})

// ---------------------------------------------------------------------------
// Azure OpenAI gpt-5.1-codex-max — Responses API extended context codex model
// ---------------------------------------------------------------------------

dangerousDescribe("Azure OpenAI - gpt-5.1-codex-max", () => {
  test("basic message", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post(
      "/v1/messages",
      {
        model: "gpt-5.1-codex-max",
        // gpt-5.1-codex-max uses ~180+ tokens for internal reasoning even for short responses
        max_tokens: 500,
        messages: [{ role: "user", content: "Reply with just the word: pong" }],
      },
      { "anthropic-version": "2023-06-01" },
    )
    console.log(
      `[TEST]✅ Basic message test (GPT-5.1-codex-max) - Status: ${status}`,
    )
    expect(status).toBe(200)
    const response = json as {
      type: string
      role: string
      content: Array<{ type: string }>
    }
    expect(response.type).toBe("message")
    expect(response.role).toBe("assistant")
    expect(response.content[0].type).toBe("text")
  }, 15000)

  test("via chat completions endpoint", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.1-codex-max",
      messages: [{ role: "user", content: "Reply with just the word: ping" }],
      max_tokens: 500,
    })
    console.log(
      `[TEST]💬 Chat completions test (GPT-5.1-codex-max) - Status: ${status}`,
    )
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (json.choices as Array<{ message: { role: string } }>)[0]
    expect(choice.message.role).toBe("assistant")
  }, 15000)

  test("TypeScript code generation tool", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")
    const { status, json } = await post("/v1/chat/completions", {
      model: "gpt-5.1-codex-max",
      messages: [
        {
          role: "user",
          content:
            "Write a TypeScript function to reverse a string. Use the code generation tool.",
        },
      ],
      tools: [OPENAI_CODE_TOOL],
      max_tokens: 300,
    })
    console.log(
      `[TEST]💻 TypeScript code generation test (GPT-5.1-codex-max) - Status: ${status}`,
    )
    expect(status).toBe(200)
    expect(json.choices).toBeDefined()
    const choice = (
      json.choices as Array<{ message: { tool_calls?: Array<any> } }>
    )[0]
    if (choice.message.tool_calls && choice.message.tool_calls.length > 0) {
      const toolNames = choice.message.tool_calls.map(
        (call) => call.function.name,
      )
      expect(toolNames).toContain("generate_typescript")
      console.log(
        `[TEST]✅ Code generation tool used by GPT-5.1-codex-max:`,
        toolNames,
      )
    } else {
      console.warn(
        "⚠️ No tools used for TypeScript generation by GPT-5.1-codex-max",
      )
    }
  }, 15000)
})

// ---------------------------------------------------------------------------
// Performance and Load Tests
// ---------------------------------------------------------------------------

dangerousDescribe("Performance", () => {
  test("concurrent tool call requests", async () => {
    await ensureProviderActive("azure-openai-jzhu-1677-resource")

    const promises = []
    const concurrency = 3

    console.log(`[TEST]🚀 Testing ${concurrency} concurrent tool call requests`)

    for (let i = 0; i < concurrency; i++) {
      const promise = post("/v1/chat/completions", {
        model: "gpt-5.4-mini",
        messages: [
          {
            role: "user",
            content: `What is the weather in city ${i}? Use the weather tool.`,
          },
        ],
        tools: [OPENAI_WEATHER_TOOL],
        max_tokens: 100,
      })
      promises.push(promise)
    }

    const results = await Promise.all(promises)
    console.log(
      `📊 Concurrent requests completed:`,
      results.map((r) => r.status),
    )

    // At least some should succeed
    const successCount = results.filter((r) => r.status === 200).length
    expect(successCount).toBeGreaterThan(0)
    console.log(
      `✅ ${successCount}/${concurrency} concurrent requests succeeded`,
    )
  }, 30000) // 30 second timeout for concurrent requests
})
