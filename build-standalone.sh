#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

FFMPEG_VERSION="${FFMPEG_VERSION:-8.0}"
EMBED_DIR="embed/ffmpeg"
OUT_DIR="dist"
LOCAL_ONLY=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --local|-l)
            LOCAL_ONLY=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --local, -l    Only build for the current platform"
            echo "  --help, -h     Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Detect current platform
CURRENT_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH=$(uname -m)
[[ "${CURRENT_ARCH}" == "x86_64" ]] && CURRENT_ARCH="amd64"
[[ "${CURRENT_ARCH}" == "aarch64" || "${CURRENT_ARCH}" == "arm64" ]] && CURRENT_ARCH="arm64"

# Platforms to build
if [[ "${LOCAL_ONLY}" == "true" ]]; then
    PLATFORMS=("${CURRENT_OS}/${CURRENT_ARCH}")
    echo "==> Building ffmpeg ${FFMPEG_VERSION} for current platform only (${CURRENT_OS}/${CURRENT_ARCH})..."
else
    PLATFORMS=(
        "linux/amd64"
        "linux/arm64"
        "darwin/arm64"
    )
    echo "==> Building ffmpeg ${FFMPEG_VERSION} for all platforms..."
fi

# Build ffmpeg for Linux platforms using Docker
build_ffmpeg_linux() {
    local arch=$1
    local output_path="${EMBED_DIR}/ffmpeg_linux_${arch}"
    
    if [[ -f "${output_path}" ]]; then
        echo "    ffmpeg_linux_${arch} already exists, skipping..."
        return
    fi
    
    echo "    Building ffmpeg_linux_${arch} via Docker..."
    
    docker buildx build \
        --platform "linux/${arch}" \
        --target ffmpeg-builder \
        --output "type=local,dest=.tmp-ffmpeg-${arch}" \
        --build-arg FFMPEG_VERSION="${FFMPEG_VERSION}" \
        -f Dockerfile \
        .
    
    cp ".tmp-ffmpeg-${arch}/opt/ffmpeg/bin/ffmpeg" "${output_path}"
    rm -rf ".tmp-ffmpeg-${arch}"
    
    echo "    Built ffmpeg_linux_${arch} ($(du -h "${output_path}" | cut -f1))"
}

# Build ffmpeg for the current platform natively
build_ffmpeg_native() {
    local output_path="${EMBED_DIR}/ffmpeg_${CURRENT_OS}_${CURRENT_ARCH}"
    
    if [[ -f "${output_path}" ]]; then
        echo "    ffmpeg_${CURRENT_OS}_${CURRENT_ARCH} already exists, skipping..."
        return
    fi
    
    # Install build dependencies on macOS
    if [[ "$(uname -s)" == "Darwin" ]]; then
        if ! command -v brew &> /dev/null; then
            echo "    ERROR: Homebrew not found. Install from https://brew.sh"
            exit 1
        fi
        brew list nasm &>/dev/null || brew install nasm
        brew list pkg-config &>/dev/null || brew install pkg-config
        brew list lame &>/dev/null || brew install lame
    fi
    
    "${SCRIPT_DIR}/scripts/build-ffmpeg.sh" "${output_path}"
}

# Ensure embed directory exists
mkdir -p "${EMBED_DIR}"
mkdir -p "${OUT_DIR}"

# Build ffmpeg for each platform
echo ""
echo "==> Building ffmpeg binaries..."
for platform in "${PLATFORMS[@]}"; do
    os="${platform%/*}"
    arch="${platform#*/}"
    if [[ "${os}" == "${CURRENT_OS}" && "${arch}" == "${CURRENT_ARCH}" ]]; then
        # Native build for current platform
        build_ffmpeg_native
    elif [[ "${os}" == "linux" ]]; then
        # Cross-compile Linux via Docker
        build_ffmpeg_linux "${arch}"
    else
        echo "    Skipping ${os}/${arch} - can only build natively on that platform"
    fi
done

# Initialize go module if needed
if [[ ! -f "go.mod" ]]; then
    echo "    Initializing Go module..."
    go mod init github.com/Fields-Education/audio-extractor
    go mod tidy
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
    
    # Get version info
    VERSION="${VERSION:-dev}"
    COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
    BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
    LDFLAGS="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}"
    
    CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \
        -trimpath \
        -ldflags="${LDFLAGS}" \
        -o "${OUT_DIR}/${output_name}" \
        .
    
    echo "    Built ${output_name} ($(du -h "${OUT_DIR}/${output_name}" | cut -f1))"
done

echo ""
echo "==> Build complete!"
echo ""
ls -lh "${OUT_DIR}/"
