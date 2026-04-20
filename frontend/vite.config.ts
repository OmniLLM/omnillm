import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { resolve } from "path"
import { defineConfig } from "vite"

const serverPort = Number(process.env.GO_PORT ?? process.env.SERVER_PORT ?? 5002)
const frontendPort = Number(process.env.FRONTEND_PORT ?? 5080)
const serverBase = `http://localhost:${serverPort}`

// Only use Vite proxy in development when both frontend and backend are on localhost
const isDev = process.env.NODE_ENV !== 'production'
const useProxy = isDev && process.env.DISABLE_PROXY !== 'true'

const proxyTarget = {
  target: serverBase,
  changeOrigin: true,
  configure: (proxy: import("http-proxy").Server) => {
    proxy.on("error", (err, req, res) => {
      console.error(`❌ Vite proxy error for ${req.url}: ${err.message}`)
      console.log(`💡 Backend might be running on a different port. Frontend will auto-detect at runtime.`)

      // Send a helpful error response
      if (res && !res.headersSent) {
        res.writeHead(502, { 'Content-Type': 'application/json' })
        res.end(JSON.stringify({
          error: 'Backend server not reachable',
          hint: 'Make sure the backend server is running. Frontend will attempt auto-detection.'
        }))
      }
    })
  },
}

export default defineConfig({
  plugins: [react(), tailwindcss()],
  root: __dirname,
  base: "/admin/",
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
    },
  },
  build: {
    outDir: resolve(__dirname, "../pages/admin"),
    emptyOutDir: true,
  },
  server: {
    port: frontendPort,
    proxy: useProxy ? {
      "/api": proxyTarget,
      "/v1": proxyTarget,
      "/usage": proxyTarget,
      "/models": proxyTarget,
    } : undefined,
  },
  define: {
    __SERVER_PORT__: JSON.stringify(serverPort),
    __API_KEY__: JSON.stringify(process.env.OMNILLM_API_KEY ?? ""),
  },
})
