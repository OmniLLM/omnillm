GO  := go
BUN := bun

BUILD_DIR := .build/bin

ifeq ($(OS),Windows_NT)
  EXE := .exe
  PREPARE_BUILD_DIR := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BUILD_DIR)' | Out-Null"
else
  EXE :=
  PREPARE_BUILD_DIR := mkdir -p "$(BUILD_DIR)"
endif

OMNILLM   := $(BUILD_DIR)/omnillm$(EXE)

ifeq ($(OS),Windows_NT)
  BUILD_DESKTOP_SIDECAR := powershell -NoProfile -File scripts/build-desktop-sidecar.ps1
  UNINSTALL := powershell -NoProfile -File scripts/uninstall-binaries.ps1
else
  BUILD_DESKTOP_SIDECAR := scripts/build-desktop-sidecar.sh
  UNINSTALL := scripts/uninstall-binaries.sh
endif

.PHONY: build install uninstall build-desktop-sidecar build-desktop desktop-dev

build:
	$(PREPARE_BUILD_DIR)
	$(GO) build -o "$(OMNILLM)" .

install:
	$(GO) install .

uninstall:
	$(UNINSTALL)

build-desktop-sidecar:
	$(BUILD_DESKTOP_SIDECAR)

build-desktop: build-desktop-sidecar
	cd desktop && $(BUN) install
	cd desktop && $(BUN) run tauri build

desktop-dev: build-desktop-sidecar
	cd desktop && $(BUN) install
	cd desktop && $(BUN) run tauri dev
