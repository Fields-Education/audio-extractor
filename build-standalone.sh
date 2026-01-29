#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

FFMPEG_VERSION="${FFMPEG_VERSION:-8.0}"
EMBED_DIR="embed/ffmpeg"
OUT_DIR="dist"

# Platforms to build
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/arm64"
)

echo "==> Building ffmpeg ${FFMPEG_VERSION} for all platforms..."

# Build ffmpeg for Linux platforms using Docker
build_ffmpeg_linux() {
    local arch=$1
    local output_name="ffmpeg_linux_${arch}"
    
    if [[ -f "${EMBED_DIR}/${output_name}" ]]; then
        echo "    ${output_name} already exists, skipping..."
        return
    fi
    
    echo "    Building ${output_name}..."
    
    docker buildx build \
        --platform "linux/${arch}" \
        --target ffmpeg-builder \
        --output "type=local,dest=.tmp-ffmpeg-${arch}" \
        --build-arg FFMPEG_VERSION="${FFMPEG_VERSION}" \
        -f Dockerfile \
        .
    
    cp ".tmp-ffmpeg-${arch}/opt/ffmpeg/bin/ffmpeg" "${EMBED_DIR}/${output_name}"
    rm -rf ".tmp-ffmpeg-${arch}"
    
    echo "    Built ${output_name} ($(du -h "${EMBED_DIR}/${output_name}" | cut -f1))"
}

# Build ffmpeg for macOS using Homebrew (must run on macOS)
build_ffmpeg_darwin() {
    local output_name="ffmpeg_darwin_arm64"
    
    if [[ -f "${EMBED_DIR}/${output_name}" ]]; then
        echo "    ${output_name} already exists, skipping..."
        return
    fi
    
    if [[ "$(uname -s)" != "Darwin" ]]; then
        echo "    WARNING: Cannot build ${output_name} - not running on macOS"
        echo "    You'll need to build this on an Apple Silicon Mac"
        return
    fi
    
    echo "    Building ${output_name} (this requires Homebrew)..."
    
    # Check for required build tools
    if ! command -v brew &> /dev/null; then
        echo "    ERROR: Homebrew not found. Install from https://brew.sh"
        exit 1
    fi
    
    # Install build dependencies if needed
    brew list nasm &>/dev/null || brew install nasm
    brew list pkg-config &>/dev/null || brew install pkg-config
    brew list lame &>/dev/null || brew install lame
    
    # Build ffmpeg from source with minimal config
    FFMPEG_BUILD_DIR=$(mktemp -d)
    trap "rm -rf ${FFMPEG_BUILD_DIR}" EXIT
    
    cd "${FFMPEG_BUILD_DIR}"
    curl -L -o ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz"
    tar xf ffmpeg.tar.xz
    cd "ffmpeg-${FFMPEG_VERSION}"
    
    # Use shared configure script
    "${SCRIPT_DIR}/scripts/ffmpeg-configure.sh" \
        --prefix="${FFMPEG_BUILD_DIR}/out" \
        --extra-cflags="-I$(brew --prefix lame)/include" \
        --extra-ldflags="-L$(brew --prefix lame)/lib"
    
    make -j"$(sysctl -n hw.ncpu)"
    
    cp ffmpeg "${SCRIPT_DIR}/${EMBED_DIR}/${output_name}"
    strip "${SCRIPT_DIR}/${EMBED_DIR}/${output_name}" || true
    
    cd "${SCRIPT_DIR}"
    trap - EXIT
    rm -rf "${FFMPEG_BUILD_DIR}"
    
    echo "    Built ${output_name} ($(du -h "${EMBED_DIR}/${output_name}" | cut -f1))"
}

# Ensure embed directory exists
mkdir -p "${EMBED_DIR}"
mkdir -p "${OUT_DIR}"

# Build ffmpeg for each platform
echo ""
echo "==> Building ffmpeg binaries..."
build_ffmpeg_linux "amd64"
build_ffmpeg_linux "arm64"
build_ffmpeg_darwin

# Initialize go module if needed
if [[ ! -f "go.mod" ]]; then
    echo "    Initializing Go module..."
    go mod init github.com/Fields-Education/audio-extractor
    go mod tidy
fi

# Check that we have at least the current platform's ffmpeg
CURRENT_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH=$(uname -m)
[[ "${CURRENT_ARCH}" == "x86_64" ]] && CURRENT_ARCH="amd64"
[[ "${CURRENT_ARCH}" == "aarch64" ]] && CURRENT_ARCH="arm64"

CURRENT_FFMPEG="${EMBED_DIR}/ffmpeg_${CURRENT_OS}_${CURRENT_ARCH}"
if [[ ! -f "${CURRENT_FFMPEG}" ]]; then
    echo ""
    echo "ERROR: Missing ffmpeg for current platform: ${CURRENT_FFMPEG}"
    echo "Cannot build standalone binary without it."
    exit 1
fi

# Build Go binary for each platform (without docker tag = embedded ffmpeg)
echo ""
echo "==> Building Go binaries with embedded ffmpeg..."

for platform in "${PLATFORMS[@]}"; do
    os="${platform%/*}"
    arch="${platform#*/}"
    output_name="audio-extractor-${os}-${arch}"
    ffmpeg_file="${EMBED_DIR}/ffmpeg_${os}_${arch}"
    
    if [[ ! -f "${ffmpeg_file}" ]]; then
        echo "    Skipping ${output_name} - missing ${ffmpeg_file}"
        continue
    fi
    
    echo "    Building ${output_name}..."
    
    CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \
        -trimpath \
        -ldflags='-s -w' \
        -o "${OUT_DIR}/${output_name}" \
        .
    
    echo "    Built ${output_name} ($(du -h "${OUT_DIR}/${output_name}" | cut -f1))"
done

echo ""
echo "==> Build complete!"
echo ""
ls -lh "${OUT_DIR}/"
