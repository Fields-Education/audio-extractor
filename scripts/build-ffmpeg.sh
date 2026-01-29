#!/bin/sh
# Build ffmpeg for the current platform
# Usage: ./scripts/build-ffmpeg.sh <output-path> [extra-configure-flags...]
# Example: ./scripts/build-ffmpeg.sh ./embed/ffmpeg/ffmpeg_linux_amd64 --cc="ccache gcc"

set -eu

show_help() {
    echo "Usage: $0 <output-path> [extra-configure-flags...]"
    echo ""
    echo "Build ffmpeg for the current platform."
    echo ""
    echo "Arguments:"
    echo "  output-path     Path where the ffmpeg binary will be written"
    echo "  extra-flags     Additional flags passed to ./configure"
    echo ""
    echo "Environment variables:"
    echo "  FFMPEG_VERSION  FFmpeg version to build (default: 8.0)"
    echo "  FFMPEG_TARBALL  Path to existing tarball (skips download if set)"
    echo ""
    echo "Examples:"
    echo "  $0 ./embed/ffmpeg/ffmpeg_linux_amd64"
    echo "  $0 ./ffmpeg --cc=\"ccache gcc\""
    echo "  FFMPEG_VERSION=8.0.1 $0 ./ffmpeg"
}

if [ $# -lt 1 ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    show_help
    exit 0
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FFMPEG_VERSION="${FFMPEG_VERSION:-8.0}"
FFMPEG_TARBALL="${FFMPEG_TARBALL:-ffmpeg.tar.xz}"

OUTPUT_PATH="$1"
shift

# Create output directory if needed
mkdir -p "$(dirname "$OUTPUT_PATH")"

# Create temp build directory
BUILD_DIR=$(mktemp -d)
cleanup() {
    rm -rf "$BUILD_DIR"
}
trap cleanup EXIT

cd "$BUILD_DIR"

if [ -f "$FFMPEG_TARBALL" ]; then
    echo "==> Using tarball: ${FFMPEG_TARBALL}"
    cp "$FFMPEG_TARBALL" ffmpeg.tar.xz
else
    echo "==> Downloading ffmpeg ${FFMPEG_VERSION}..."
    curl -L -o ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz"
fi

echo "==> Extracting..."
mkdir -p ffmpeg
tar xf ffmpeg.tar.xz --strip-components=1 -C ffmpeg
cd ffmpeg

echo "==> Configuring..."
# Detect platform-specific flags
PLATFORM_FLAGS=""
case "$(uname -s)" in
    Darwin)
        # macOS needs Homebrew paths for lame
        if command -v brew >/dev/null 2>&1; then
            LAME_PREFIX="$(brew --prefix lame 2>/dev/null || echo "")"
            if [ -n "$LAME_PREFIX" ] && [ -d "$LAME_PREFIX" ]; then
                PLATFORM_FLAGS="--extra-cflags=-I${LAME_PREFIX}/include --extra-ldflags=-L${LAME_PREFIX}/lib"
            fi
        fi
        ;;
    Linux)
        # Linux static build
        PLATFORM_FLAGS="--extra-ldflags=-static"
        ;;
esac

# Run configure with shared flags + platform flags + user flags
# shellcheck disable=SC2086
"$SCRIPT_DIR/ffmpeg-configure.sh" \
    --prefix="$BUILD_DIR/out" \
    $PLATFORM_FLAGS \
    "$@"

echo "==> Building..."
case "$(uname -s)" in
    Darwin)
        make -j"$(sysctl -n hw.ncpu)"
        ;;
    *)
        make -j"$(nproc)"
        ;;
esac

echo "==> Installing to ${OUTPUT_PATH}..."
cp ffmpeg "$OUTPUT_PATH"
strip "$OUTPUT_PATH" 2>/dev/null || true

echo "==> Done! Built $(du -h "$OUTPUT_PATH" | cut -f1) binary"
