# OmniLLM Makefile
# Wraps common development and build operations.
#
# Windows Note: If 'make' is not in your PATH, add it:
#   set PATH=C:\Program Files (x86)\GnuWin32\bin;%PATH%
# Or install via: winget install GnuWin32.Make

# ── Configuration ─────────────────────────────────────────────────────────────

SERVER_PORT   ?= 5000
FRONTEND_PORT ?= 5080
HOST          ?= 127.0.0.1
REBUILD       ?=

GO            := go
BUN           := bun
DEPS          := .build/.bun-install.stamp

INSTALL_DIR   := $(HOME)/.local/bin
BUILD_DIR     := .build/bin

ifeq ($(OS),Windows_NT)
  EXE         := .exe
  INSTALL_DIR := $(USERPROFILE)/.local/bin
  PRINT_BLANK := @powershell -NoProfile -Command "Write-Host ''"
  TOUCH_TARGET = powershell -NoProfile -Command "New-Item -ItemType File -Force -Path '$(1)' | Out-Null"
  MKDIR_TARGET = powershell -NoProfile -Command "if (-not (Test-Path '$(1)')) { New-Item -ItemType Directory -Path '$(1)' | Out-Null }"
  BUILD_DESKTOP_SIDECAR = powershell -NoProfile -File scripts/build-desktop-sidecar.ps1
  CLEAN_DESKTOP_SIDECAR = powershell -NoProfile -Command "Get-ChildItem 'desktop/src-tauri/target' -Recurse -Filter 'omniproxy-*$(EXE)' -ErrorAction SilentlyContinue | Remove-Item -Force -ErrorAction SilentlyContinue"
else
  EXE         :=
  PRINT_BLANK := @printf '\n'
  TOUCH_TARGET = touch "$(1)"
  define MKDIR_TARGET
	mkdir -p "$(1)"
  endef
  define BUILD_DESKTOP_SIDECAR
	@TARGET=$$($(GO) env GOOS)-$$($(GO) env GOARCH); \
	HOST_TRIPLE=$$(rustc -vV 2>/dev/null | sed -n 's/^host: //p'); \
	if [ -z "$$HOST_TRIPLE" ]; then \
	  echo "rustc not found; cannot build desktop sidecar"; exit 1; \
	fi; \
	OUT="desktop/src-tauri/binaries/omniproxy-$$HOST_TRIPLE$(EXE)"; \
	echo "Building $$OUT"; \
	$(GO) build -o "$$OUT" ./cmd/omniproxy
  endef
endif

OMNILLM_BIN   := $(INSTALL_DIR)/omnillm$(EXE)
OMNIPROXY_BIN := $(INSTALL_DIR)/omniproxy$(EXE)

# ── Phony targets ─────────────────────────────────────────────────────────────

.PHONY: all install deps build build-go build-frontend build-all \
        build-desktop-sidecar build-desktop desktop-dev \
        start start-redis stop restart restart-rebuild status logs logs-follow \
        dev dev-frontend \
        test test-frontend lint typecheck \
        release-patch release-minor release-major \
        docker-build docker-run \
        help help-redis-examples

# ── Default ───────────────────────────────────────────────────────────────────

all: help

# ── Install ───────────────────────────────────────────────────────────────────

## install: Build all Go binaries and install to ~/.local/bin
install:
	$(call MKDIR_TARGET,$(INSTALL_DIR))
	$(GO) build -o "$(OMNILLM_BIN)" .
	$(GO) build -o "$(OMNIPROXY_BIN)" ./cmd/omniproxy
	@echo Installed omnillm$(EXE) to $(OMNILLM_BIN)
	@echo Installed omniproxy$(EXE) to $(OMNIPROXY_BIN)

## deps: Install Node.js dependencies with Bun
deps: $(DEPS)

$(DEPS): bun.lock package.json
	$(call MKDIR_TARGET,$(BUILD_DIR))
	$(BUN) install
	@$(call TOUCH_TARGET,$(DEPS))

## build: Build all Go binaries and install to ~/.local/bin
build: build-go

## build-go: Compile all Go binaries and install to ~/.local/bin
build-go:
	$(call MKDIR_TARGET,$(INSTALL_DIR))
	$(GO) build -o "$(OMNILLM_BIN)" .
	$(GO) build -o "$(OMNIPROXY_BIN)" ./cmd/omniproxy
	@echo Built omnillm$(EXE) to $(OMNILLM_BIN)
	@echo Built omniproxy$(EXE) to $(OMNIPROXY_BIN)

## build-frontend: Build the frontend assets (outputs to pages/)
build-frontend: $(DEPS)
	$(BUN) run build

## build-all: Build all Go binaries and the frontend assets
build-all: build-go build-frontend

# ── Desktop (Tauri) ───────────────────────────────────────────────────────────

## build-desktop-sidecar: Build omniproxy as the Tauri sidecar binary
build-desktop-sidecar:
	$(call MKDIR_TARGET,desktop/src-tauri/binaries)
	$(CLEAN_DESKTOP_SIDECAR)
	$(BUILD_DESKTOP_SIDECAR)

## build-desktop: Build the Tauri desktop app (sidecar + frontend + tauri build)
build-desktop: build-desktop-sidecar
	cd desktop && $(BUN) install
	cd desktop && $(BUN) run tauri build

## desktop-dev: Run the desktop app in development mode (hot reload)
desktop-dev: build-desktop-sidecar
	cd desktop && $(BUN) install
	cd desktop && $(BUN) run tauri dev

# ── Dev / Start ───────────────────────────────────────────────────────────────

## start: Build the Go backend and start all services in the background
start: $(DEPS)
	$(BUN) run omni start --server-port $(SERVER_PORT) --frontend-port $(FRONTEND_PORT) --host $(HOST)

## start-redis: Start all services with an existing Redis response-cache backend
start-redis: start

## stop: Stop all background services
stop: $(DEPS)
	$(BUN) run omni stop

## restart: Restart background services (no rebuild)
restart: $(DEPS)
	$(BUN) run omni restart $(REBUILD) --server-port $(SERVER_PORT) --frontend-port $(FRONTEND_PORT) --host $(HOST)

## restart-rebuild: Rebuild everything and restart background services
restart-rebuild: $(DEPS)
	$(BUN) run omni restart --rebuild --server-port $(SERVER_PORT) --frontend-port $(FRONTEND_PORT) --host $(HOST)

## status: Show running service status and ports
status: $(DEPS)
	$(BUN) run omni status

## logs: Print the last 50 lines of service logs
logs: $(DEPS)
	$(BUN) run omni logs

## logs-follow: Stream service logs in real time
logs-follow: $(DEPS)
	$(BUN) run omni logs --follow

## dev: Start both backend and frontend in the foreground (Ctrl+C to stop)
dev: $(DEPS)
	$(BUN) run dev --server-port $(SERVER_PORT) --frontend-port $(FRONTEND_PORT) --host $(HOST)

## dev-frontend: Start only the Vite frontend dev server
dev-frontend: $(DEPS)
	$(BUN) run dev:frontend

# ── Test ──────────────────────────────────────────────────────────────────────

## test: Run the full test suite
test: $(DEPS)
	$(BUN) run test

## test-frontend: Run frontend tests only
test-frontend: $(DEPS)
	$(BUN) run test:frontend

# ── Code Quality ──────────────────────────────────────────────────────────────

## lint: Run ESLint on changed files
lint: $(DEPS)
	$(BUN) run lint

## typecheck: Run TypeScript type checking on the frontend
typecheck: $(DEPS)
	$(BUN) run typecheck

# ── Release ───────────────────────────────────────────────────────────────────

## release-patch: Bump patch version, commit, tag, and push
release-patch: $(DEPS)
	$(BUN) run scripts/release.ts patch

## release-minor: Bump minor version, commit, tag, and push
release-minor: $(DEPS)
	$(BUN) run scripts/release.ts minor

## release-major: Bump major version, commit, tag, and push
release-major: $(DEPS)
	$(BUN) run scripts/release.ts major

# ── Docker ────────────────────────────────────────────────────────────────────

## docker-build: Build the Docker image tagged as omnillm
docker-build:
	docker build -t omnillm .

## docker-run: Run the Docker image on port 5002
docker-run:
	docker run -p $(SERVER_PORT):5002 omnillm

# ── Help ──────────────────────────────────────────────────────────────────────

## help: List all available targets with descriptions
help:
	@echo OmniLLM Development Tasks
	@echo Usage: make [target] [VARIABLE=value ...]
	$(PRINT_BLANK)
	@echo VARIABLES:
	@echo   SERVER_PORT=5000       Backend API port - default 5000
	@echo   FRONTEND_PORT=5080     Frontend dev server port - default 5080
	@echo   HOST=127.0.0.1         Bind address - default 127.0.0.1
	@echo   OMNILLM_RESPONSE_CACHE_REDIS_URL
	@echo                         Redis endpoint - default redis://127.0.0.1:6379/0
	@echo   REBUILD=--rebuild      Add --rebuild flag to restart target
	$(PRINT_BLANK)
	@echo QUICK START:
	@echo   start                Build the Go backend and start all services in the background
	@echo   start-redis          Start all services with an existing Redis response-cache backend
	@echo   stop                 Stop all background services
	@echo   dev                  Start both backend and frontend in the foreground - Ctrl+C to stop
	@echo   status               Show running service status and ports
	$(PRINT_BLANK)
	@echo BUILD:
	@echo   deps                 Install Node.js dependencies with Bun
	@echo   install              Build all Go binaries and install to ~/.local/bin
	@echo   build                Build all Go binaries: omnillm and omniproxy - install to ~/.local/bin
	@echo   build-go             Compile all Go binaries: omnillm and omniproxy - install to ~/.local/bin
	@echo   build-frontend       Build the frontend assets - outputs to pages/
	@echo   build-all            Build all Go binaries and the frontend assets
	@echo   build-desktop        Build Tauri desktop app: bundles omniproxy as sidecar
	@echo   desktop-dev          Run Tauri desktop app in dev mode
	$(PRINT_BLANK)
	@echo DEVELOPMENT:
	@echo   dev-frontend         Start only the Vite frontend dev server
	@echo   restart              Restart background services - no rebuild
	@echo   restart-rebuild      Rebuild everything and restart background services
	$(PRINT_BLANK)
	@echo MONITORING:
	@echo   logs                 Print the last 50 lines of service logs
	@echo   logs-follow          Stream service logs in real time
	$(PRINT_BLANK)
	@echo TESTING and QUALITY:
	@echo   test                 Run the full test suite
	@echo   test-frontend        Run frontend tests only
	@echo   lint                 Run ESLint on changed files
	@echo   typecheck            Run TypeScript type checking on the frontend
	$(PRINT_BLANK)
	@echo RELEASE:
	@echo   release-patch        Bump patch version, commit, tag, and push
	@echo   release-minor        Bump minor version, commit, tag, and push
	@echo   release-major        Bump major version, commit, tag, and push
	$(PRINT_BLANK)
	@echo DOCKER:
	@echo   docker-build         Build the Docker image tagged as omnillm
	@echo   docker-run           Run the Docker image on port 5000
	$(PRINT_BLANK)
	@echo EXAMPLES:
	@echo   make deps
	@echo   make dev
	@echo   make start SERVER_PORT=5000 FRONTEND_PORT=5080
	@echo   make start-redis
	@$(MAKE) --no-print-directory help-redis-examples
	@echo   omnillm settings set response-cache on --ttl 3600
	@echo   make restart REBUILD=--rebuild
	@echo   make logs-follow

help-redis-examples:
	$(info   POSIX: OMNILLM_RESPONSE_CACHE_REDIS_URL=redis://redis.example:6379/0 make start-redis)
	$(info   PowerShell: $$env:OMNILLM_RESPONSE_CACHE_REDIS_URL='redis://redis.example:6379/0'; make start-redis)
	$(info   cmd.exe: set "OMNILLM_RESPONSE_CACHE_REDIS_URL=redis://redis.example:6379/0" && make start-redis)
	@:
