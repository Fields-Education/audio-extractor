#!/bin/sh
# Shared ffmpeg configure flags for audio-extractor
# This script runs ./configure with the minimal flags needed for audio extraction
#
# Usage: ./scripts/ffmpeg-configure.sh [extra-flags...]
# Example: ./scripts/ffmpeg-configure.sh --prefix=/opt/ffmpeg --cc="ccache gcc"

set -eu

./configure \
    --disable-debug \
    --disable-doc \
    --disable-ffplay \
    --disable-ffprobe \
    --disable-everything \
    --disable-network \
    --enable-static \
    --disable-shared \
    --enable-small \
    \
    --enable-libmp3lame \
    \
    --enable-protocol=file \
    --enable-protocol=pipe \
    \
    --enable-demuxer=matroska \
    --enable-demuxer=webm \
    --enable-demuxer=mov \
    --enable-demuxer=mp4 \
    --enable-demuxer=ogg \
    --enable-demuxer=flac \
    --enable-demuxer=wav \
    --enable-demuxer=mp3 \
    --enable-demuxer=aac \
    \
    --enable-decoder=aac \
    --enable-decoder=aac_latm \
    --enable-decoder=alac \
    --enable-decoder=mp3 \
    --enable-decoder=mp3float \
    --enable-decoder=mp3on4 \
    --enable-decoder=mp3on4float \
    --enable-decoder=vorbis \
    --enable-decoder=opus \
    --enable-decoder=flac \
    --enable-decoder=pcm_s16le \
    --enable-decoder=pcm_s16be \
    --enable-decoder=pcm_s24le \
    --enable-decoder=pcm_s24be \
    --enable-decoder=pcm_s32le \
    --enable-decoder=pcm_s32be \
    --enable-decoder=pcm_f32le \
    --enable-decoder=pcm_f32be \
    --enable-decoder=pcm_f64le \
    --enable-decoder=pcm_f64be \
    \
    --enable-encoder=pcm_s16le \
    --enable-encoder=libmp3lame \
    --enable-encoder=flac \
    \
    --enable-muxer=wav \
    --enable-muxer=mp3 \
    --enable-muxer=flac \
    \
    --enable-filter=aresample \
    --enable-filter=aformat \
    --enable-filter=highpass \
    --enable-filter=lowpass \
    --enable-filter=afftdn \
    --enable-filter=adeclick \
    --enable-filter=deesser \
    --enable-filter=dynaudnorm \
    \
    --enable-parser=aac \
    --enable-parser=mp3 \
    --enable-parser=vorbis \
    --enable-parser=opus \
    --enable-parser=flac \
    \
    "$@"
