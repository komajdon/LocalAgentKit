#!/usr/bin/env bash
# Build ai-agent for all platforms and pack each into a zip archive.
# Produces two variants per platform:
#   ai-agent-<platform>-full.zip   — app + whisper binary + GGML model
#   ai-agent-<platform>-lite.zip   — app only (whisper downloaded at runtime)
#
# Windows cross-compilation from Linux requires mingw-w64:
#   sudo apt install gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64
# On Windows, run the script under Git Bash or WSL.
#
# Usage: ./scripts/release.sh [version]
# Example: ./scripts/release.sh v1.0.0
set -euo pipefail

VERSION="${1:-dev}"
DIST="dist/${VERSION}"
BIN="ai-agent"
WHISPER_VERSION="1.7.4"
WHISPER_MODEL="base"
WHISPER_SRC_URL="https://github.com/ggerganov/whisper.cpp/archive/refs/tags/v${WHISPER_VERSION}.tar.gz"
WHISPER_MODEL_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-${WHISPER_MODEL}.bin"
# Pre-built whisper.cpp Windows release (no MSVC needed)
WHISPER_WIN_URL="https://github.com/ggerganov/whisper.cpp/releases/download/v${WHISPER_VERSION}/whisper-bin-x64.zip"

BOLD='\033[1m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[0;33m'
RESET='\033[0m'

step()  { echo -e "\n${BOLD}${CYAN}▶ $*${RESET}"; }
ok()    { echo -e "${GREEN}✓ $*${RESET}"; }
warn()  { echo -e "${YELLOW}⚠ $*${RESET}"; }

require() {
    for cmd in "$@"; do
        command -v "$cmd" >/dev/null 2>&1 || { echo "ERROR: '$cmd' not found — please install it first."; exit 1; }
    done
}

OS="$(uname -s)"
mkdir -p "$DIST"

# ── Whisper GGML model (shared across all platforms) ─────────────────────────
step "Whisper GGML model (${WHISPER_MODEL})"
MODEL_CACHE="build/bin/data/models/ggml-${WHISPER_MODEL}.bin"
mkdir -p "$(dirname "$MODEL_CACHE")"
if [ ! -f "$MODEL_CACHE" ]; then
    echo "  Downloading ggml-${WHISPER_MODEL}.bin (~74 MB)…"
    wget -q --show-progress -O "$MODEL_CACHE" "$WHISPER_MODEL_URL"
fi
ok "Model ready: $MODEL_CACHE"

# ── Compile whisper.cpp for the host (Linux / macOS) ─────────────────────────
WHISPER_BIN_UNIX="build/bin/whisper"
if command -v cmake >/dev/null 2>&1; then
    step "Compiling whisper.cpp v${WHISPER_VERSION} (host)"
    WHISPER_BUILD_DIR="/tmp/whisper-build"
    WHISPER_SRC="${WHISPER_BUILD_DIR}/whisper.cpp-${WHISPER_VERSION}"

    mkdir -p "$WHISPER_BUILD_DIR"
    if [ ! -d "$WHISPER_SRC" ]; then
        echo "  Downloading source…"
        wget -q --show-progress -O "${WHISPER_BUILD_DIR}/whisper.tar.gz" "$WHISPER_SRC_URL"
        tar -xf "${WHISPER_BUILD_DIR}/whisper.tar.gz" -C "$WHISPER_BUILD_DIR"
        rm "${WHISPER_BUILD_DIR}/whisper.tar.gz"
    fi

    cmake -S "$WHISPER_SRC" -B "${WHISPER_SRC}/build" \
        -DCMAKE_BUILD_TYPE=Release \
        -DWHISPER_BUILD_TESTS=OFF \
        -DWHISPER_BUILD_EXAMPLES=ON \
        -Wno-dev 2>/dev/null

    cmake --build "${WHISPER_SRC}/build" --target whisper-cli \
        -j"$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)" 2>/dev/null

    mkdir -p build/bin
    cp "${WHISPER_SRC}/build/bin/whisper-cli" "$WHISPER_BIN_UNIX"
    ok "whisper (unix) binary ready: $WHISPER_BIN_UNIX"
else
    warn "cmake not found — Unix whisper binary skipped (lite-only for Linux/macOS)"
    WHISPER_BIN_UNIX=""
fi

# ── Fetch pre-built whisper.cpp Windows binary ────────────────────────────────
WHISPER_BIN_WIN=""
fetch_whisper_windows() {
    local cache_dir="/tmp/whisper-win-${WHISPER_VERSION}"
    local zip_path="${cache_dir}/whisper-bin-x64.zip"
    local cli_path="${cache_dir}/main/whisper-cli.exe"

    mkdir -p "$cache_dir"
    if [ ! -f "$cli_path" ]; then
        echo "  Downloading whisper.cpp Windows binary…"
        wget -q --show-progress -O "$zip_path" "$WHISPER_WIN_URL"
        unzip -q "$zip_path" -d "$cache_dir"
    fi

    if [ -f "$cli_path" ]; then
        echo "$cli_path"
    else
        # Try any .exe in the zip
        find "$cache_dir" -name "whisper-cli.exe" | head -1
    fi
}

# ── Helper: pack a built artifact into lite + full zips ──────────────────────
# pack_zips <name> <app_bin_path> <whisper_bin_or_empty>
pack_zips() {
    local name="$1"
    local app_bin="$2"
    local whisper_bin="$3"

    # lite (app only)
    local lite_dir="${DIST}/${name}-lite"
    mkdir -p "$lite_dir"
    [ -f "$app_bin" ] && cp "$app_bin" "${lite_dir}/"
    (cd "$lite_dir" && zip -r "../${name}-lite.zip" . -x "*.zip")
    ok "Packed → ${DIST}/${name}-lite.zip"

    # full (app + whisper + model)
    local full_dir="${DIST}/${name}-full"
    mkdir -p "${full_dir}/data/models"
    [ -f "$app_bin" ] && cp "$app_bin" "${full_dir}/"
    cp "$MODEL_CACHE" "${full_dir}/data/models/"
    if [ -n "$whisper_bin" ] && [ -f "$whisper_bin" ]; then
        local whisper_dst="whisper"
        # keep .exe extension on Windows binaries
        [[ "$whisper_bin" == *.exe ]] && whisper_dst="whisper.exe"
        cp "$whisper_bin" "${full_dir}/${whisper_dst}"
        (cd "$full_dir" && zip -r "../${name}-full.zip" . -x "*.zip")
        ok "Packed → ${DIST}/${name}-full.zip"
    else
        warn "whisper binary unavailable — full variant skipped for ${name}"
    fi
}

# ── Linux ─────────────────────────────────────────────────────────────────────
step "Building Linux (amd64)"
require wails go zip wget
wails build -platform linux/amd64 -tags webkit2_41 -o "${BIN}-linux-amd64" 2>/dev/null
pack_zips "${BIN}-linux-amd64" "build/bin/${BIN}-linux-amd64" "$WHISPER_BIN_UNIX"

# ── macOS ─────────────────────────────────────────────────────────────────────
if [[ "$OS" == "Darwin" ]]; then
    step "Building macOS (amd64)"
    wails build -platform darwin/amd64 -o "${BIN}-darwin-amd64" 2>/dev/null
    pack_zips "${BIN}-darwin-amd64" "build/bin/${BIN}-darwin-amd64" "$WHISPER_BIN_UNIX"

    step "Building macOS (arm64)"
    wails build -platform darwin/arm64 -o "${BIN}-darwin-arm64" 2>/dev/null
    pack_zips "${BIN}-darwin-arm64" "build/bin/${BIN}-darwin-arm64" "$WHISPER_BIN_UNIX"
fi

# ── Windows ───────────────────────────────────────────────────────────────────
build_windows() {
    step "Building Windows (amd64)"

    local can_cross=false
    # On Linux: cross-compile with mingw-w64
    if [[ "$OS" == "Linux" ]]; then
        if command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then
            can_cross=true
        else
            warn "mingw-w64 not found. Install with:"
            warn "  sudo apt install gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64"
            warn "Skipping Windows build."
            return
        fi
    fi

    # Build the .exe
    # On Windows (Git Bash/WSL) just call wails directly.
    # On Linux cross-compile via CC override.
    # CGO_ENABLED=0 skips webkit2gtk (Linux-only) headers which aren't
    # available in the mingw toolchain; Wails embeds the WebView2 runtime
    # on Windows and does not need webkit at build time.
    if [[ "$OS" == "Linux" ]] && $can_cross; then
        CC=x86_64-w64-mingw32-gcc \
        CXX=x86_64-w64-mingw32-g++ \
        CGO_ENABLED=1 \
        GOOS=windows GOARCH=amd64 \
        wails build -platform windows/amd64 -o "${BIN}-windows-amd64.exe" 2>/dev/null
    else
        wails build -platform windows/amd64 -o "${BIN}-windows-amd64.exe" 2>/dev/null
    fi

    # Fetch whisper Windows binary
    local win_whisper
    win_whisper=$(fetch_whisper_windows) || true

    pack_zips "${BIN}-windows-amd64" \
        "build/bin/${BIN}-windows-amd64.exe" \
        "${win_whisper:-}"
}

build_windows

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
step "Archives in ${DIST}/"
find "${DIST}" -maxdepth 1 -name "*.zip" \
    | sort \
    | while read -r f; do
        size=$(du -sh "$f" 2>/dev/null | cut -f1)
        printf "  %-50s %s\n" "$(basename "$f")" "$size"
      done
echo ""
ok "Done — version ${VERSION}"
