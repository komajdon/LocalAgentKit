.PHONY: dev build build-linux build-windows build-mac-intel build-mac-arm \
        bundle-whisper whisper-bin-linux whisper-bin-mac whisper-bin-windows \
        download-whisper-model package-deb package-rpm package-linux \
        package-dmg package-windows clean

BIN             := ai-agent
VERSION         ?= 0.0.0-dev
WHISPER_VERSION := 1.7.4
WHISPER_MODEL   := base
WHISPER_URL     := https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-$(WHISPER_MODEL).bin
WHISPER_SRC_URL := https://github.com/ggerganov/whisper.cpp/archive/refs/tags/v$(WHISPER_VERSION).tar.gz

# ── Development ───────────────────────────────────────────────────────────────

dev:
	wails dev -tags webkit2_41

# ── Platform builds ───────────────────────────────────────────────────────────

build-linux: bundle-whisper
	wails build -platform linux/amd64 -tags webkit2_41 -o $(BIN)-linux-amd64

build-windows:
	wails build -platform windows/amd64 -o $(BIN)-windows-amd64.exe

# Cross-compile for Windows from Linux (requires mingw-w64)
build-windows-cross:
	CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ \
	CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
	wails build -platform windows/amd64 -o $(BIN)-windows-amd64.exe

build-mac-intel: bundle-whisper
	wails build -platform darwin/amd64 -o $(BIN)-darwin-amd64

build-mac-arm: bundle-whisper
	wails build -platform darwin/arm64 -o $(BIN)-darwin-arm64

# Build for the current platform only
build: bundle-whisper
	wails build -tags webkit2_41

# Build all platforms (requires matching OS for macOS/Windows)
build-all: build-linux build-windows build-mac-intel build-mac-arm

# ── Whisper bundling ──────────────────────────────────────────────────────────

# Download the GGML model and compile the whisper.cpp binary for the current platform.
bundle-whisper: download-whisper-model whisper-bin-linux

# Compile whisper.cpp from source (Linux / macOS).
# Produces build/bin/whisper — placed next to the app binary.
whisper-bin-linux:
	@echo "→ Compiling whisper.cpp v$(WHISPER_VERSION)…"
	@mkdir -p /tmp/whisper-build && cd /tmp/whisper-build && \
	  ([ -d whisper.cpp-$(WHISPER_VERSION) ] || \
	    (wget -q $(WHISPER_SRC_URL) -O whisper.tar.gz && tar -xf whisper.tar.gz && rm whisper.tar.gz)) && \
	  cmake -S whisper.cpp-$(WHISPER_VERSION) -B whisper.cpp-$(WHISPER_VERSION)/build \
	    -DCMAKE_BUILD_TYPE=Release -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=ON \
	    -DCMAKE_VERBOSE_MAKEFILE=OFF -Wno-dev 2>/dev/null && \
	  cmake --build whisper.cpp-$(WHISPER_VERSION)/build --target whisper-cli -j$(shell nproc) 2>/dev/null
	@mkdir -p build/bin
	@cp /tmp/whisper-build/whisper.cpp-$(WHISPER_VERSION)/build/bin/whisper-cli build/bin/whisper
	@echo "✓ whisper binary → build/bin/whisper"

whisper-bin-mac:
	@echo "→ Compiling whisper.cpp v$(WHISPER_VERSION) (macOS)…"
	@mkdir -p /tmp/whisper-build && cd /tmp/whisper-build && \
	  ([ -d whisper.cpp-$(WHISPER_VERSION) ] || \
	    (curl -sL $(WHISPER_SRC_URL) -o whisper.tar.gz && tar -xf whisper.tar.gz && rm whisper.tar.gz)) && \
	  cmake -S whisper.cpp-$(WHISPER_VERSION) -B whisper.cpp-$(WHISPER_VERSION)/build \
	    -DCMAKE_BUILD_TYPE=Release -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=ON -Wno-dev 2>/dev/null && \
	  cmake --build whisper.cpp-$(WHISPER_VERSION)/build --target whisper-cli -j$(shell sysctl -n hw.ncpu) 2>/dev/null
	@mkdir -p build/bin
	@cp /tmp/whisper-build/whisper.cpp-$(WHISPER_VERSION)/build/bin/whisper-cli build/bin/whisper
	@echo "✓ whisper binary → build/bin/whisper"

# Download the GGML model into the data directory that ships with the app.
download-whisper-model:
	@mkdir -p build/bin/data/models
	@if [ ! -f build/bin/data/models/ggml-$(WHISPER_MODEL).bin ]; then \
	  echo "→ Downloading whisper $(WHISPER_MODEL) model (~74 MB)…"; \
	  wget -q --show-progress -O build/bin/data/models/ggml-$(WHISPER_MODEL).bin $(WHISPER_URL) && \
	  echo "✓ Model → build/bin/data/models/ggml-$(WHISPER_MODEL).bin"; \
	else \
	  echo "✓ Model already present"; \
	fi

# ── OS packages ───────────────────────────────────────────────────────────────
# Produce native installers so non-technical users avoid Gatekeeper / SmartScreen
# and get menu entries + icons. Run the matching build-* target first.

# .deb + .rpm via nfpm (https://nfpm.goreleaser.com). Requires `nfpm` on PATH:
#   go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest
package-deb:
	@command -v nfpm >/dev/null || { echo "nfpm not found: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest"; exit 1; }
	@test -f build/bin/$(BIN)-linux-amd64 || { echo "missing build/bin/$(BIN)-linux-amd64 — run 'make build-linux'"; exit 1; }
	@mkdir -p dist
	VERSION=$(VERSION) nfpm package -f build/nfpm.yaml -p deb -t dist/
	@echo "✓ .deb → dist/"

package-rpm:
	@command -v nfpm >/dev/null || { echo "nfpm not found: go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest"; exit 1; }
	@test -f build/bin/$(BIN)-linux-amd64 || { echo "missing build/bin/$(BIN)-linux-amd64 — run 'make build-linux'"; exit 1; }
	@mkdir -p dist
	VERSION=$(VERSION) nfpm package -f build/nfpm.yaml -p rpm -t dist/
	@echo "✓ .rpm → dist/"

package-linux: package-deb package-rpm

# macOS .dmg from the built .app bundle (run on macOS after 'make build-mac-arm').
# Set APPLE_DEVELOPER_ID / APPLE_NOTARY_PROFILE to sign + notarise.
package-dmg:
	@app=$$(find build/bin -maxdepth 1 -name '*.app' | head -1); \
	  test -n "$$app" || { echo "no .app bundle in build/bin — run 'make build-mac-arm'"; exit 1; }; \
	  build/darwin/make-dmg.sh "$$app" "dist/$(BIN)-$(VERSION).dmg"

# Windows installer (NSIS) via Wails — produces build/bin/*-installer.exe.
package-windows:
	wails build -platform windows/amd64 -nsis -o $(BIN)-windows-amd64.exe -ldflags "-X main.Version=$(VERSION)"

# ── Misc ─────────────────────────────────────────────────────────────────────

clean:
	rm -rf build/bin/ dist/
