import { describe, expect, test } from "bun:test"

import type { AuthFlow, Provider } from "../../frontend/src/api"

import { getDeviceAuthCopy } from "../../frontend/src/lib/device-auth"

describe("getDeviceAuthCopy", () => {
  const alibabaProvider: Provider = {
    id: "alibaba-2",
    type: "alibaba",
    name: "Alibaba DashScope",
    isActive: false,
    authStatus: "unauthenticated",
  }

  const githubProvider: Provider = {
    id: "github-copilot-1",
    type: "github-copilot",
    name: "GitHub Copilot",
    isActive: false,
    authStatus: "unauthenticated",
  }

  test("treats Alibaba URLs with embedded user codes as prefilled", () => {
    const authFlow: AuthFlow = {
      providerId: "alibaba-2",
      status: "awaiting_user",
      userCode: "JGPYTR1R",
      instructionURL:
        "https://chat.qwen.ai/authorize?user_code=JGPYTR1R&client=qwen-code&prompt=login",
    }

    const copy = getDeviceAuthCopy(authFlow, [alibabaProvider])

    expect(copy.codeLabel).toBe("This code is already filled in for Qwen:")
    expect(copy.codeHint).toContain("confirm the login")
    expect(copy.waitingLabel).toContain("confirm in the browser")
  })

  test("keeps manual-entry copy for providers that need device codes entered", () => {
    const authFlow: AuthFlow = {
      providerId: "github-copilot-1",
      status: "awaiting_user",
      userCode: "ABCD-1234",
      instructionURL: "https://github.com/login/device",
    }

    const copy = getDeviceAuthCopy(authFlow, [githubProvider])

    expect(copy.codeLabel).toBe("Enter this code:")
    expect(copy.codeHint).toBeUndefined()
    expect(copy.waitingLabel).toBe("Waiting for authorization…")
  })
})
