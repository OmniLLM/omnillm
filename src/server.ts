import { Hono } from "hono"
import { serveStatic } from "hono/bun"
import { cors } from "hono/cors"
import consola from "consola"
import { crypto } from "node:crypto"

import { adminRoutes } from "./routes/admin/route"
import { completionRoutes } from "./routes/chat-completions/route"
import { embeddingRoutes } from "./routes/embeddings/route"
import { messageRoutes } from "./routes/messages/route"
import { modelRoutes } from "./routes/models/route"
import { responsesRoutes } from "./routes/responses/route"
import { tokenRoute } from "./routes/token/route"
import { usageRoute } from "./routes/usage/route"

export const server = new Hono()

// Request ID middleware
server.use("*", async (c, next) => {
  const requestId = crypto.randomUUID()
  c.set("requestId", requestId)
  c.header("X-Request-Id", requestId)
  await next()
})

// Custom logger middleware with source location tracking
server.use("*", async (c, next) => {
  const start = Date.now()
  const method = c.req.method
  const path = c.req.path
  const requestId = c.get("requestId")

  // Log incoming request
  consola.info(`<-- ${method} ${path} [${requestId}]`)

  await next()

  // Log response
  const ms = Date.now() - start
  const status = c.res.status
  consola.info(`--> ${method} ${path} ${status} ${ms}ms [${requestId}]`)
})

server.use(cors())

server.get("/", (c) => c.text("Server running"))

server.route("/chat/completions", completionRoutes)
server.route("/models", modelRoutes)
server.route("/embeddings", embeddingRoutes)
server.route("/usage", usageRoute)
server.route("/token", tokenRoute)

// Compatibility with tools that expect v1/ prefix
server.route("/v1/chat/completions", completionRoutes)
server.route("/v1/models", modelRoutes)
server.route("/v1/embeddings", embeddingRoutes)

// Anthropic compatible endpoints
server.route("/v1/messages", messageRoutes)

// Responses API endpoints
server.route("/v1/responses", responsesRoutes)
server.route("/responses", responsesRoutes)

// Admin API
server.route("/api/admin", adminRoutes)

// Admin SPA
server.get("/admin", (c) => c.redirect("/admin/"))
server.use(
  "/admin/*",
  serveStatic({
    root: "./pages/admin",
    rewriteRequestPath: (p) => p.replace(/^\/admin/, ""),
  }),
)
