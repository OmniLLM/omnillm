/* eslint-disable @typescript-eslint/no-unsafe-call,@typescript-eslint/no-unsafe-member-access */
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { resolve } from "node:path"
import { defineConfig } from "vite"

const serverPort = Number(
  process.env.GO_PORT ?? process.env.SERVER_PORT ?? 5002,
)
const frontendPort = Number(process.env.FRONTEND_PORT ?? 5080)
const serverBase = `http://localhost:${serverPort}`

// Only use Vite proxy in development when both frontend and backend are on localhost
const isDev = process.env.NODE_ENV !== "production"
const useProxy = isDev && process.env.DISABLE_PROXY !== "true"

const proxyTarget = {
  target: serverBase,
  changeOrigin: true,
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  configure: (proxy: any) => {
    proxy.on(
      "error",
      (
        err: unknown,
        req: { url?: string },
        res: {
          headersSent?: boolean
          writeHead?: (status: number, headers: Record<string, string>) => void
          end?: (body: string) => void
        },
      ) => {
        const message = err instanceof Error ? err.message : String(err)
        console.error(`❌ Vite proxy error for ${req.url}: ${message}`)
        console.log(
          `💡 Backend might be running on a different port. Frontend will auto-detect at runtime.`,
        )

        // Send a helpful error response
        if (res.writeHead && res.end && !res.headersSent) {
          res.writeHead(502, { "Content-Type": "application/json" })
          res.end(
            JSON.stringify({
              error: "Backend server not reachable",
              hint: "Make sure the backend server is running. Frontend will attempt auto-detection.",
            }),
          )
        }
      },
    )
  },
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  root: import.meta.dirname,
  base: "/admin/",
  resolve: {
    alias: {
      "@": resolve(import.meta.dirname, "src"),
    },
  },
  build: {
    outDir: resolve(import.meta.dirname, "../pages/admin"),
    emptyOutDir: true,
  },
  server: {
    port: frontendPort,
    proxy:
      useProxy ?
        {
          "/api": proxyTarget,
          "/v1": proxyTarget,
          "/usage": proxyTarget,
          "/models": proxyTarget,
        }
      : undefined,
  },
  define: {
    __SERVER_PORT__: JSON.stringify(serverPort),
    __API_KEY__: JSON.stringify(process.env.OMNILLM_API_KEY ?? ""),
  },
})
