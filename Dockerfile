# ── Build stage: compile Go binary ─────────────────────────────────────────
FROM golang:1.23-alpine AS go-builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o omnillm main.go

# ── Build stage: build frontend assets ─────────────────────────────────────
FROM oven/bun:1.2.19-alpine AS frontend-builder
WORKDIR /app

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY . .
RUN bun run build

# ── Runner stage ─────────────────────────────────────────────────────────────
FROM alpine:3.20 AS runner
WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=go-builder /app/omnillm ./omnillm
COPY --from=frontend-builder /app/pages ./pages

EXPOSE 5002

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget --spider -q http://localhost:5002/ || exit 1

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
