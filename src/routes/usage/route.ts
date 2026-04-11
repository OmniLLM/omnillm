import { Hono } from "hono"
import consola from "consola"

import { state } from "~/lib/state"
import { getCopilotUsage } from "~/services/github/get-copilot-usage"

export const usageRoute = new Hono()

usageRoute.get("/", async (c) => {
  try {
    const provider = state.currentProvider
    if (provider && provider.id !== "github-copilot") {
      const response = await provider.getUsage()
      return response
    }
    const usage = await getCopilotUsage()
    return c.json(usage)
  } catch (error) {
    consola.error("Error fetching usage:", error)
    return c.json({ error: "Failed to fetch usage" }, 500)
  }
})
