/**
 * Frontend Integration Tests for Provider Management UI
 *
 * Tests realistic user workflows and edge cases
 *
 * Run with: bun test tests/provider-ui-workflows.test.ts
 *
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
    formatDangerousTestSkipMessage("tests/provider-ui-workflows.test.ts"),
  )
}

// Helper functions
async function get(path: string) {
  const res = await fetch(`${BASE}${path}`, {
    headers: { "Content-Type": "application/json" },
  })
  const data = await res.json()
  if (!res.ok) {
    throw new Error(
      `GET ${path} failed: ${res.status} - ${JSON.stringify(data)}`,
    )
  }
  return data
}

async function post(path: string, body: unknown = {}) {
  const res = await fetch(`${BASE}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  })
  const data = await res.json()
  if (!res.ok) {
    throw new Error(
      `POST ${path} failed: ${res.status} - ${JSON.stringify(data)}`,
    )
  }
  return data
}

async function del(path: string) {
  const res = await fetch(`${BASE}${path}`, {
    method: "DELETE",
    headers: { "Content-Type": "application/json" },
  })
  const data = await res.json()
  if (!res.ok) {
    throw new Error(
      `DELETE ${path} failed: ${res.status} - ${JSON.stringify(data)}`,
    )
  }
  return data
}

dangerousDescribe("Provider Management UI Workflows", () => {
  let testProviders: Array<string> = []

  beforeAll(async () => {
    const server = await startUITestServer()
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
    testProviders = []
  })

  afterEach(async () => {
    await Promise.allSettled(
      testProviders.map(async (providerId) => {
        try {
          await del(`/api/admin/providers/${providerId}`)
        } catch {
          // Best-effort cleanup; provider may have already been deleted.
        }
      }),
    )
    testProviders = []
  })

  describe("Complete Provider Lifecycle", () => {
    test("should complete full provider lifecycle: create → auth → activate → deactivate → delete", async () => {
      // Step 1: Create provider
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      expect(createResult.success).toBe(true)
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      // Verify provider exists and is unauthenticated
      let providers = await get("/api/admin/providers")
      let provider = providers.find((p) => p.id === providerId)
      expect(provider).toBeDefined()
      expect(provider.authStatus).toBe("unauthenticated")
      expect(provider.isActive).toBe(false)

      // Step 2: Attempt authentication (will fail with test credentials)
      try {
        await post(`/api/admin/providers/${providerId}/auth`, {
          method: "token",
          token: "ghu_invalid_test_token",
        })
      } catch (error) {
        // Expected - test token is invalid
        expect(error.message).toContain("500")
      }

      // Step 3: Activate provider (should work even without valid auth)
      const activateResult = await post(
        `/api/admin/providers/${providerId}/activate`,
      )
      expect(activateResult.success).toBe(true)

      // Verify activation
      providers = await get("/api/admin/providers")
      provider = providers.find((p) => p.id === providerId)
      expect(provider.isActive).toBe(true)

      // Step 4: Deactivate provider
      const deactivateResult = await post(
        `/api/admin/providers/${providerId}/deactivate`,
      )
      expect(deactivateResult.success).toBe(true)

      // Verify deactivation
      providers = await get("/api/admin/providers")
      provider = providers.find((p) => p.id === providerId)
      expect(provider.isActive).toBe(false)

      // Step 5: Delete provider
      const deleteResult = await del(`/api/admin/providers/${providerId}`)
      expect(deleteResult.success).toBe(true)

      // Verify deletion
      providers = await get("/api/admin/providers")
      provider = providers.find((p) => p.id === providerId)
      expect(provider).toBeUndefined()

      // Remove from cleanup list since we deleted it
      testProviders = testProviders.filter((id) => id !== providerId)
    })
  })

  describe("Multi-Provider Scenarios", () => {
    test("should handle multiple providers of same type", async () => {
      // Create multiple GitHub Copilot instances
      const provider1 = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const provider2 = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )

      testProviders.push(provider1.provider.id, provider2.provider.id)

      // Verify both exist
      const providers = await get("/api/admin/providers")
      const githubProviders = providers.filter(
        (p) => p.type === "github-copilot",
      )
      expect(githubProviders.length).toBeGreaterThanOrEqual(2)

      // Verify they have different IDs
      expect(provider1.provider.id).not.toBe(provider2.provider.id)
    })

    test("should handle provider priority management with multiple active providers", async () => {
      // Create multiple providers
      const alibaba1 = await post("/api/admin/providers/alibaba/add-instance")
      const alibaba2 = await post("/api/admin/providers/alibaba/add-instance")

      testProviders.push(alibaba1.provider.id, alibaba2.provider.id)

      // Activate both
      await post(`/api/admin/providers/${alibaba1.provider.id}/activate`)
      await post(`/api/admin/providers/${alibaba2.provider.id}/activate`)

      // Set priorities
      const priorities = {
        [alibaba1.provider.id]: 1,
        [alibaba2.provider.id]: 2,
      }

      const priorityResult = await post("/api/admin/providers/priorities", {
        priorities,
      })
      expect(priorityResult.success).toBe(true)

      // Verify priorities
      const savedPriorities = await get("/api/admin/providers/priorities")
      expect(savedPriorities.priorities[alibaba1.provider.id]).toBe(1)
      expect(savedPriorities.priorities[alibaba2.provider.id]).toBe(2)
    })
  })

  describe("Error Recovery Scenarios", () => {
    test("should handle auth flow interruption gracefully", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      // Start OAuth flow (will fail but tests the flow)
      try {
        await post(`/api/admin/providers/${providerId}/auth`, {
          method: "oauth", // This should trigger OAuth flow
        })
      } catch {
        // Expected to fail in test environment
      }

      // Check auth status
      const authStatus = await get("/api/admin/auth-status")
      // Should either be null or have error status
      if (authStatus) {
        expect(["pending", "awaiting_user", "complete", "error"]).toContain(
          authStatus.status,
        )
      }
    })

    test("should handle concurrent operations gracefully", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      // Try multiple operations concurrently
      const operations = [
        post(`/api/admin/providers/${providerId}/activate`),
        get(`/api/admin/providers/${providerId}/models`).catch((e) => ({
          error: e.message,
        })),
        get(`/api/admin/providers/${providerId}/usage`).catch((e) => ({
          error: e.message,
        })),
      ]

      const results = await Promise.all(operations)

      // At least the activation should succeed
      expect(results[0]).toHaveProperty("success", true)
    })
  })

  describe("UI Edge Cases", () => {
    test("should handle empty provider list scenario", async () => {
      const providers = await get("/api/admin/providers")
      expect(providers).toEqual([])
    })

    test("should handle provider with zero models", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      try {
        const models = await get(`/api/admin/providers/${providerId}/models`)
        expect(models).toHaveProperty("models")
        expect(Array.isArray(models.models)).toBe(true)
      } catch (error) {
        // Expected for unauthenticated provider
        expect(error.message).toContain("500")
      }
    })

    test("should validate model toggle operations", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      // Test invalid model toggle request
      try {
        await post(`/api/admin/providers/${providerId}/models/toggle`, {
          // Missing modelId
          enabled: true,
        })
      } catch (error) {
        expect(error.message).toContain("400")
      }

      // Test valid model toggle request
      const result = await post(
        `/api/admin/providers/${providerId}/models/toggle`,
        {
          modelId: "test-model-id",
          enabled: false,
        },
      )
      expect(result).toHaveProperty("success", true)
      expect(result).toHaveProperty("enabled", false)
    })
  })

  describe("Data Consistency", () => {
    test("should maintain consistent state across operations", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/antigravity/add-instance",
      )
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      // Get initial state
      let providers = await get("/api/admin/providers")
      let provider = providers.find((p) => p.id === providerId)
      expect(provider.isActive).toBe(false)

      // Activate
      await post(`/api/admin/providers/${providerId}/activate`)

      // Verify state changed
      providers = await get("/api/admin/providers")
      provider = providers.find((p) => p.id === providerId)
      expect(provider.isActive).toBe(true)

      // Get status - should reflect the change
      const status = await get("/api/admin/status")
      expect(typeof status.modelCount).toBe("number")
    })

    test("should handle provider deletion cleanup", async () => {
      // Create provider
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id

      // Activate it
      await post(`/api/admin/providers/${providerId}/activate`)

      // Set some model preferences
      await post(`/api/admin/providers/${providerId}/models/toggle`, {
        modelId: "test-model",
        enabled: false,
      })

      // Delete provider
      await del(`/api/admin/providers/${providerId}`)

      // Verify complete cleanup
      const providers = await get("/api/admin/providers")
      const deletedProvider = providers.find((p) => p.id === providerId)
      expect(deletedProvider).toBeUndefined()

      // Status should be consistent
      const status = await get("/api/admin/status")
      expect(typeof status.modelCount).toBe("number")
    })
  })

  describe("Authentication Flow Edge Cases", () => {
    test("should handle malformed auth requests", async () => {
      const createResult = await post(
        "/api/admin/providers/github-copilot/add-instance",
      )
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      // Only token-based requests are validated synchronously for GitHub Copilot.
      const malformedRequests = [
        { method: "token" }, // Missing token
        { method: "token", token: "" }, // Empty token
      ]

      for (const request of malformedRequests) {
        try {
          await post(`/api/admin/providers/${providerId}/auth`, request)
          expect(true).toBe(false) // Should fail
        } catch (error) {
          const status = Number.parseInt(
            error.message.match(/failed: (\d+) -/)?.[1] || "0",
            10,
          )
          // 400/500 = malformed request; 409 = auth flow already in progress from prior test
          expect([400, 409, 500]).toContain(status)
        }
      }
    })

    test("should handle concurrent auth requests", async () => {
      const createResult = await post(
        "/api/admin/providers/alibaba/add-instance",
      )
      const providerId = createResult.provider.id
      testProviders.push(providerId)

      // Try multiple auth requests simultaneously
      const authRequests = [
        post(`/api/admin/providers/${providerId}/auth`, {
          method: "api-key",
          apiKey: "sk-test1",
          region: "global",
        }).catch((e) => ({ error: e.message })),
        post(`/api/admin/providers/${providerId}/auth`, {
          method: "api-key",
          apiKey: "sk-test2",
          region: "china",
        }).catch((e) => ({ error: e.message })),
      ]

      const results = await Promise.all(authRequests)

      // Both should fail (invalid keys) but not crash
      for (const result of results) {
        expect(result).toHaveProperty("error")
      }
    })
  })
})
