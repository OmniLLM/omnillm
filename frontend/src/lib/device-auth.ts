import type { AuthFlow, Provider } from "@/api"

type DeviceAuthCopy = {
  codeLabel: string
  codeHint?: string
  waitingLabel: string
}

function normalizeUserCode(value: string): string {
  return value.replaceAll(/[^a-z0-9]/gi, "").toUpperCase()
}

function hasEmbeddedUserCode(authFlow: AuthFlow | null | undefined): boolean {
  if (!authFlow?.instructionURL || !authFlow.userCode) {
    return false
  }

  try {
    const parsedURL = new URL(authFlow.instructionURL)
    const embeddedCode = parsedURL.searchParams.get("user_code")
    const normalizedEmbeddedCode =
      typeof embeddedCode === "string" ? normalizeUserCode(embeddedCode) : null

    return (
      normalizedEmbeddedCode !== null
      && normalizedEmbeddedCode === normalizeUserCode(authFlow.userCode)
    )
  } catch {
    return false
  }
}

export function getDeviceAuthCopy(
  authFlow: AuthFlow | null | undefined,
  providers: Array<Provider>,
): DeviceAuthCopy {
  const provider = providers.find((entry) => entry.id === authFlow?.providerId)
  const isAlibabaFlow = provider?.type === "alibaba"
  const codeIsPrefilled = hasEmbeddedUserCode(authFlow)

  if (!isAlibabaFlow) {
    return {
      codeLabel: "Enter this code:",
      waitingLabel: "Waiting for authorization…",
    }
  }

  return {
    codeLabel:
      codeIsPrefilled ?
        "This code is already filled in for Qwen:"
      : "Use this code if Qwen asks for it:",
    codeHint:
      codeIsPrefilled ?
        "Open the page and confirm the login. You only need to type the code if Qwen prompts for it."
      : "Qwen may take you straight to a confirmation page. Only enter the code if the site asks for it.",
    waitingLabel: "Waiting for authorization after you confirm in the browser…",
  }
}
