#!/bin/sh
# Download ffmpeg source tarball
# Usage: ./scripts/download-ffmpeg.sh [output-path]
# Example: ./scripts/download-ffmpeg.sh ./cache/ffmpeg-8.0.tar.xz

set -eu

FFMPEG_VERSION="${FFMPEG_VERSION:-8.0}"
OUTPUT_PATH="${1:-ffmpeg-${FFMPEG_VERSION}.tar.xz}"

if [ -f "$OUTPUT_PATH" ]; then
    echo "==> Tarball already exists: ${OUTPUT_PATH}"
    exit 0
fi

mkdir -p "$(dirname "$OUTPUT_PATH")"

echo "==> Downloading ffmpeg ${FFMPEG_VERSION} to ${OUTPUT_PATH}..."
curl -L -o "$OUTPUT_PATH" "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz"
echo "==> Done! $(du -h "$OUTPUT_PATH" | cut -f1)"
