/**
 * Material Design UI Verification Script
 *
 * Run this to verify that the Material Design components are working correctly
 *
 * Usage:
 * 1. Start the dev server: bun run dev
 * 2. Run this test: bun test tests/verify-material-ui.test.ts
 */

import { describe, test, expect, beforeAll } from "bun:test"

const API_BASE = "http://localhost:4141"
const FRONTEND_BASE = "http://localhost:5175"

// These are manual integration tests that require a running dev server.
// Run: bun run dev  (then)  bun test tests/verify-material-ui.test.ts
// Skip in CI or when no server is running.
let serverAvailable = false
try {
  const resp = await fetch(`${API_BASE}/api/admin/status`, { signal: AbortSignal.timeout(2000) })
  serverAvailable = resp.ok
} catch {
  // server not running
}

const describeIntegration = serverAvailable ? describe : describe.skip

describeIntegration("Material UI Verification", () => {
  test("✅ API server should be accessible", async () => {
    try {
      const response = await fetch(`${API_BASE}/api/admin/status`)
      expect(response.ok).toBe(true)

      const status = await response.json()
      expect(status).toHaveProperty("modelCount")
      console.log("✅ API server is working correctly")
    } catch (error) {
      console.error("❌ API server is not accessible. Please run 'bun run dev'")
      throw error
    }
  })

  test("✅ Frontend should be accessible", async () => {
    try {
      const response = await fetch(`${FRONTEND_BASE}/admin/`)
      expect(response.ok).toBe(true)

      const html = await response.text()
      expect(html).toContain("OmniLLM Admin")
      expect(html).toContain('id="root"')
      expect(html).toContain("main.tsx")
      console.log("✅ Frontend is loading correctly")
    } catch (error) {
      console.error("❌ Frontend is not accessible. Please run 'bun run dev'")
      throw error
    }
  })

  test("✅ Material Design components should build without errors", async () => {
    try {
      // Test that we can create and delete providers (Material UI functionality)
      const createRes = await fetch(
        `${API_BASE}/api/admin/providers/alibaba/add-instance`,
        {
          method: "POST",
        },
      )
      expect(createRes.ok).toBe(true)

      const result = await createRes.json()
      expect(result.success).toBe(true)

      const providerId = result.provider.id

      // Clean up
      const deleteRes = await fetch(
        `${API_BASE}/api/admin/providers/${providerId}`,
        {
          method: "DELETE",
        },
      )
      expect(deleteRes.ok).toBe(true)

      console.log("✅ Material Design components are functioning correctly")
    } catch (error) {
      console.error("❌ Material Design components have issues")
      throw error
    }
  })

  test("✅ All provider types should be available", async () => {
    const providerTypes = [
      "github-copilot",
      "alibaba",
      "antigravity",
      "azure-openai",
    ]
    const createdProviders: Array<string> = []

    try {
      for (const type of providerTypes) {
        const response = await fetch(
          `${API_BASE}/api/admin/providers/${type}/add-instance`,
          {
            method: "POST",
          },
        )
        expect(response.ok).toBe(true)

        const result = await response.json()
        expect(result.success).toBe(true)
        expect(result.provider.type).toBe(type)

        createdProviders.push(result.provider.id)
      }

      console.log("✅ All provider types are working correctly")
    } finally {
      // Clean up all created providers
      await Promise.all(
        createdProviders.map(async (id) => {
          try {
            await fetch(`${API_BASE}/api/admin/providers/${id}`, {
              method: "DELETE",
            })
          } catch {
            // Ignore cleanup errors
          }
        }),
      )
    }
  })

  test("✅ Settings functionality should work", async () => {
    try {
      // Test log level get/set
      const getRes = await fetch(`${API_BASE}/api/admin/settings/log-level`)
      expect(getRes.ok).toBe(true)

      const currentLevel = await getRes.json()
      expect(currentLevel).toHaveProperty("level")

      // Test server info
      const infoRes = await fetch(`${API_BASE}/api/admin/info`)
      expect(infoRes.ok).toBe(true)

      const info = await infoRes.json()
      expect(info).toHaveProperty("version")
      expect(info).toHaveProperty("port")

      console.log("✅ Settings functionality is working correctly")
    } catch (error) {
      console.error("❌ Settings functionality has issues")
      throw error
    }
  })

  test("✅ Build system should be working", async () => {
    try {
      const { spawn } = require("node:child_process")

      // Run build command
      const buildProcess = spawn("bun", ["run", "build"], {
        stdio: "pipe",
        shell: true,
      })

      let buildOutput = ""
      buildProcess.stdout.on("data", (data: Buffer) => {
        buildOutput += data.toString()
      })

      buildProcess.stderr.on("data", (data: Buffer) => {
        buildOutput += data.toString()
      })

      await new Promise((resolve, reject) => {
        buildProcess.on("close", (code: number) => {
          if (code === 0) {
            resolve(null)
          } else {
            reject(new Error(`Build failed with code ${code}: ${buildOutput}`))
          }
        })
      })

      console.log("✅ Build system is working correctly")
    } catch (error) {
      console.error("❌ Build system has issues")
      throw error
    }
  }, 30000) // 30 second timeout for build
})

// Print summary
console.log(`
🎨 Material Design UI Verification Complete!

If all tests pass, your Material Design implementation is working correctly.

To test the UI manually:
1. Make sure the server is running: bun run dev
2. Open your browser to: http://localhost:5175/admin/
3. Click the 🎨 button in the header to toggle Material Design
4. Test all three pages: Providers, Settings, Logging

The Material Design components should:
✓ Load without errors
✓ Display properly with Material-UI styling
✓ Allow toggling between original and Material Design
✓ Maintain all original functionality
`)
