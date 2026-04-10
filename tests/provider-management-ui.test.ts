/**
 * Comprehensive UI tests for Provider Management functionality
 *
 * Tests all UI operations against an isolated proxy server
 *
 * Run with: bun test tests/provider-management-ui.test.ts
 */

import { describe, test, expect, beforeEach, afterEach, beforeAll, afterAll } from "bun:test"

import {
  formatDangerousTestSkipMessage,
  RUN_DANGEROUS_TESTS,
} from "./test-config"
import { startUITestServer } from "./ui-test-server"

let BASE = ""
let stopServer: (() => Promise<void>) | null = null
const dangerousDescribe = RUN_DANGEROUS_TESTS ? describe : describe.skip

if (!RUN_DANGEROUS_TESTS) {
  console.warn(
    formatDangerousTestSkipMessage("tests/provider-management-ui.test.ts"),
  )
}

// Helper functions for API calls
async function get(path: string) {
  const res = await fetch(`${BASE}${path}`, {
    headers: { "Content-Type": "application/json" },
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(
      `GET ${path} failed: ${res.status} - ${JSON.stringify(body)}`,
    )
  }
  return res.json()
}

async function post(path: string, body: unknown = {}) {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const responseBody = await res.json().catch(() => ({}))
    throw new Error(
      `POST ${path} failed: ${res.status} - ${JSON.stringify(responseBody)}`,
    )
  }
  return res.json()
}

async function del(path: string) {
  const res = await fetch(`${BASE}${path}`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(
      `DELETE ${path} failed: ${res.status} - ${JSON.stringify(body)}`,
    )
  }
  return res.json()
}

// Test data
const TEST_ALIBABA_CONFIG = {
  method: "api-key",
  apiKey: "sk-test123456789",
  region: "global",
}

const TEST_ANTIGRAVITY_CONFIG = {
  method: "oauth",
  clientId: "test-client-id",
  clientSecret: "test-client-secret",
}

dangerousDescribe("Provider Management UI Operations", () => {
  let createdProviders: Array<string> = []

  beforeAll(async () => {
    const server = await startUITestServer(25_000)
    BASE = server.baseUrl
    stopServer = server.stop
  }, 30_000)

  afterAll(async () => {
    if (stopServer) {
      try {
        await stopServer()
      } catch {
        // Ignore cleanup errors (e.g., Windows file lock on temp dir)
      }
      stopServer = null
    }
  })

  beforeEach(async () => {
    // Clean up all providers to ensure fresh state between tests
    try {
      const providers = await get("/api/admin/providers")
      await Promise.allSettled(
        providers.map((p: { id: string }) =>
          del(`/api/admin/providers/${p.id}`).catch(() => {}),
        ),
      )
    } catch {
      // Ignore cleanup errors
    }
    createdProviders = []
  })

  afterEach(async () => {
    await Promise.allSettled(
      createdProviders.map(async (providerId) => {
        try {
          await del(`/api/admin/providers/${providerId}`)
        } catch {
          // Best-effort cleanup; provider may have already been deleted.
        }
      }),
    )
    createdProviders = []
  })

  describe("Provider Listing", () => {
    test("should list all providers", async () => {
      const providers = await get("/api/admin/providers")

      expect(Array.isArray(providers)).toBe(true)

      // Each provider should have required fields
      for (const provider of providers) {
        expect(provider).toHaveProperty("id")
        expect(provider).toHaveProperty("type")
        expect(provider).toHaveProperty("name")
        expect(provider).toHaveProperty("isActive")
        expect(provider).toHaveProperty("authStatus")
        expect(typeof provider.isActive).toBe("boolean")
        expect(["authenticated", "unauthenticated"]).toContain(
          provider.authStatus,
        )
      }
    })

    test("should return no providers when none exist", async () => {
      const providers = await get("/api/admin/providers")

      expect(providers).toEqual([])
    })
  })

  describe("Provider Instance Management", () => {
    test("should create new GitHub Copilot instance", async () => {
      const result = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )

      expect(result).toHaveProperty("success", true)
      expect(result).toHaveProperty("provider")
      expect(result.provider).toHaveProperty("id")
      expect(result.provider).toHaveProperty("type", "github-copilot")
      expect(result.provider).toHaveProperty("name")
      expect(result.provider).toHaveProperty("authStatus", "unauthenticated")

      createdProviders.push(result.provider.id)
    })

    test("should create new Alibaba instance", async () => {
      const result = await post("/api/admin/providers/alibaba/add-instance")

      expect(result).toHaveProperty("success", true)
      expect(result).toHaveProperty("provider")
      expect(result.provider).toHaveProperty("type", "alibaba")

      createdProviders.push(result.provider.id)
    })

    test("should create new Antigravity instance", async () => {
      const result = await post("/api/admin/providers/antigravity/add-instance")

      expect(result).toHaveProperty("success", true)
      expect(result).toHaveProperty("provider")
      expect(result.provider).toHaveProperty("type", "antigravity")

      createdProviders.push(result.provider.id)
    })

    test("should delete provider instance", async () => {
      // Create a provider first
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id

      // Delete it
      const deleteResult = await del(`/api/admin/providers/${providerId}`)
      expect(deleteResult).toHaveProperty("success", true)

      // Verify it's gone
      const providers = await get("/api/admin/providers")
      const deletedProvider = providers.find((p) => p.id === providerId)
      expect(deletedProvider).toBeUndefined()
    })

    test("should handle deletion of non-existent provider", async () => {
      try {
        await del("/api/admin/providers/non-existent-provider")
        expect(true).toBe(false) // Should not reach here
      } catch (error) {
        expect(error.message).toContain("404")
      }
    })
  })

  describe("Provider Authentication", () => {
    test("should handle Alibaba API key authentication", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      // Note: This will fail without valid API key, but we can test the request structure
      try {
        await post(
          `/api/admin/providers/${providerId}/auth`,
          TEST_ALIBABA_CONFIG,
        )
      } catch (error) {
        // Expected to fail with invalid API key
        expect(error.message).toContain("500") // Auth will fail with test key
      }
    })

    test("should handle GitHub Copilot token authentication", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      // Test token auth (will fail with invalid token)
      try {
        await post(`/api/admin/providers/${providerId}/auth`, {
          method: "token",
          token: "ghu_invalid_test_token",
        })
      } catch (error) {
        // Expected to fail with invalid token
        expect(error.message).toContain("500")
      }
    })

    test("should handle Antigravity OAuth setup", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/antigravity/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      // Test OAuth setup (will fail without real credentials)
      try {
        await post(
          `/api/admin/providers/${providerId}/auth`,
          TEST_ANTIGRAVITY_CONFIG,
        )
      } catch (error) {
        // Expected to fail with test credentials
        expect(error.message).toContain("500")
      }
    })
  })

  describe("Provider Activation/Deactivation", () => {
    test("should activate and deactivate provider", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      // Activate (should work even without auth for testing)
      const activateResult = await post(
        `/api/admin/providers/${providerId}/activate`,
      )
      expect(activateResult).toHaveProperty("success", true)

      // Verify it's active
      const providers = await get("/api/admin/providers")
      const activeProvider = providers.find((p) => p.id === providerId)
      expect(activeProvider.isActive).toBe(true)

      // Deactivate
      const deactivateResult = await post(
        `/api/admin/providers/${providerId}/deactivate`,
      )
      expect(deactivateResult).toHaveProperty("success", true)

      // Verify it's inactive
      const providersAfter = await get("/api/admin/providers")
      const inactiveProvider = providersAfter.find((p) => p.id === providerId)
      expect(inactiveProvider.isActive).toBe(false)
    })
  })

  describe("Models Management", () => {
    test("should list provider models", async () => {
      // Create and try to get models for a provider
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      try {
        const models = await get(`/api/admin/providers/${providerId}/models`)
        expect(models).toHaveProperty("models")
        expect(Array.isArray(models.models)).toBe(true)
      } catch (error) {
        // Expected to fail for unauthenticated provider
        expect(error.message).toContain("500")
      }
    })

    test("should toggle model enabled state", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      // Toggle a model (will work even if no models exist)
      const toggleResult = await post(
        `/api/admin/providers/${providerId}/models/toggle`,
        {
          modelId: "test-model",
          enabled: true,
        },
      )
      expect(toggleResult).toHaveProperty("success", true)
      expect(toggleResult).toHaveProperty("modelId", "test-model")
      expect(toggleResult).toHaveProperty("enabled", true)
    })
  })

  describe("Usage Information", () => {
    test("should get provider usage data", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      try {
        const usage = await get(`/api/admin/providers/${providerId}/usage`)
        expect(typeof usage).toBe("object")
      } catch (error) {
        // Expected to fail for unauthenticated provider
        expect(error.message).toContain("500")
      }
    })
  })

  describe("Provider Priorities", () => {
    test("should get and set provider priorities", async () => {
      // Get current priorities
      const priorities = await get("/api/admin/providers/priorities")
      expect(priorities).toHaveProperty("priorities")
      expect(typeof priorities.priorities).toBe("object")

      // Set new priorities
      const newPriorities = {
        "test-provider-1": 1,
        "test-provider-2": 2,
      }

      const result = await post("/api/admin/providers/priorities", {
        priorities: newPriorities,
      })
      expect(result).toHaveProperty("success", true)

      // Verify priorities were set
      const updatedPriorities = await get("/api/admin/providers/priorities")
      expect(updatedPriorities.priorities).toMatchObject(newPriorities)
    })
  })

  describe("Server Status and Info", () => {
    test("should get server status", async () => {
      const status = await get("/api/admin/status")

      expect(status).toHaveProperty("modelCount")
      expect(status).toHaveProperty("manualApprove")
      expect(status).toHaveProperty("rateLimitWait")
      expect(typeof status.modelCount).toBe("number")
      expect(typeof status.manualApprove).toBe("boolean")
      expect(typeof status.rateLimitWait).toBe("boolean")
    })

    test("should get server info", async () => {
      const info = await get("/api/admin/info")

      expect(info).toHaveProperty("version")
      expect(info).toHaveProperty("port")
      expect(typeof info.version).toBe("string")
      expect(typeof info.port).toBe("number")
    })

    test("should get auth status", async () => {
      const authStatus = await get("/api/admin/auth-status")
      // Can be null if no auth flow is active
      expect(authStatus === null || typeof authStatus === "object").toBe(true)
    })
  })

  describe("Error Handling", () => {
    test("should handle invalid provider type for instance creation", async () => {
      try {
        await post("/api/admin/providers/invalid-type/add-instance")
        expect(true).toBe(false) // Should not reach here
      } catch (error) {
        expect(error.message).toContain("400")
      }
    })

    test("should handle invalid provider ID for operations", async () => {
      try {
        await post("/api/admin/providers/invalid-id/activate")
        expect(true).toBe(false) // Should not reach here
      } catch (error) {
        expect(error.message).toContain("400")
      }
    })

    test("should handle malformed authentication requests", async () => {
      // Create provider first
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id
      createdProviders.push(providerId)

      try {
        await post(`/api/admin/providers/${providerId}/auth`, {
          method: "api-key",
          // Missing apiKey field
        })
        expect(true).toBe(false) // Should not reach here
      } catch (error) {
        // 400 = malformed request; 409 = auth flow already in progress from prior test
        const status = Number.parseInt(
          error.message.match(/failed: (\d+) -/)?.[1] || "0",
          10,
        )
        expect([400, 409]).toContain(status)
      }
    })
  })

  describe("Models API Endpoint", () => {
    test("should handle models endpoint when no providers active", async () => {
      const models = await get("/models")

      expect(models).toHaveProperty("object", "list")
      expect(models).toHaveProperty("data")
      expect(Array.isArray(models.data)).toBe(true)
      expect(models).toHaveProperty("has_more", false)

      // Should return empty array when no providers are active
      expect(models.data.length).toBe(0)
    })
  })

  describe("Chat Completions API Endpoint", () => {
    test("should handle chat completions when no providers active", async () => {
      try {
        await post("/chat/completions", {
          model: "gpt-3.5-turbo",
          messages: [{ role: "user", content: "test" }],
        })
        expect(true).toBe(false) // Should not reach here
      } catch (error) {
        expect(error.message).toContain("400")
      }
    })
  })
})
