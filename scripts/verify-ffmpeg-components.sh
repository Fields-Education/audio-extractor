#!/bin/sh
# Verify that the minimal FFmpeg build includes every component used by the
# service's conversion and poster extraction commands.

set -eu

FFMPEG_BIN="${1:-ffmpeg}"

if [ ! -x "$FFMPEG_BIN" ]; then
    echo "ERROR: ffmpeg binary is not executable: ${FFMPEG_BIN}" >&2
    exit 1
fi

if ! "$FFMPEG_BIN" -hide_banner -version >/dev/null 2>&1; then
    echo "ERROR: ffmpeg binary could not be executed: ${FFMPEG_BIN}" >&2
    exit 1
fi

check_table_component() {
    kind="$1"
    name="$2"

    if "$FFMPEG_BIN" -hide_banner "-${kind}" 2>/dev/null |
        awk -v name="$name" '
            NF >= 2 {
                split($2, aliases, ",")
                for (i in aliases) {
                    if (aliases[i] == name) {
                        found = 1
                    }
                }
            }
            END { exit found ? 0 : 1 }
        '; then
        return
    fi

    echo "ERROR: missing FFmpeg ${kind%?}: ${name}" >&2
    exit 1
}

check_protocol() {
    name="$1"

    if "$FFMPEG_BIN" -hide_banner -protocols 2>/dev/null |
        awk -v name="$name" '$1 == name { found = 1 } END { exit found ? 0 : 1 }'; then
        return
    fi

    echo "ERROR: missing FFmpeg protocol: ${name}" >&2
    exit 1
}

for protocol in file pipe; do
    check_protocol "$protocol"
done

for demuxer in matroska webm mov mp4 ogg flac wav mp3 aac; do
    check_table_component demuxers "$demuxer"
done

for decoder in \
    aac aac_latm alac mp3 mp3float mp3on4 mp3on4float vorbis opus \
    h264 hevc mpeg4 vp8 vp9 av1 \
    flac pcm_s16le pcm_s16be pcm_s24le pcm_s24be pcm_s32le pcm_s32be \
    pcm_f32le pcm_f32be pcm_f64le pcm_f64be; do
    check_table_component decoders "$decoder"
done

for encoder in pcm_s16le libmp3lame flac mjpeg; do
    check_table_component encoders "$encoder"
done

for muxer in wav mp3 flac image2pipe; do
    check_table_component muxers "$muxer"
done

for filter in \
    blackframe metadata format scale \
    aresample aformat highpass lowpass afftdn adeclick deesser dynaudnorm; do
    check_table_component filters "$filter"
done

echo "FFmpeg component verification passed: ${FFMPEG_BIN}"
