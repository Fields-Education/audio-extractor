# syntax=docker/dockerfile:1.6

# --- Stage 1: Build ffmpeg (audio-only minimal) ---
# Must build on target platform since FFmpeg can't easily cross-compile
FROM --platform=${BUILDPLATFORM} alpine:latest AS ffmpeg-builder
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
RUN apk add --no-cache \
  ca-certificates curl git build-base pkgconf nasm yasm automake autoconf libtool zlib-dev zlib-static binutils ccache

ARG FFMPEG_VERSION=8.0
ENV PREFIX=/opt/ffmpeg
ENV CCACHE_DIR=/root/.cache/ccache
ENV PATH=/usr/lib/ccache:$PATH
RUN mkdir -p /build
WORKDIR /build
RUN curl -L -o ffmpeg.tar.xz https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz
RUN tar xf ffmpeg.tar.xz
WORKDIR /build/ffmpeg-${FFMPEG_VERSION}
RUN --mount=type=cache,target=/root/.cache/ccache CC="ccache gcc" CXX="ccache g++" ./configure \
  --disable-debug \
  --disable-doc \
  --disable-ffplay \
  --disable-ffprobe \
  --disable-everything \
  --disable-network \
  --enable-static \
  --disable-shared \
  --enable-small \
  --enable-protocol=pipe \
  --enable-demuxer=matroska \
  --enable-demuxer=webm \
  --enable-demuxer=mov \
  --enable-demuxer=mp4 \
  --enable-demuxer=ogg \
  --enable-demuxer=flac \
  --enable-demuxer=wav \
  --enable-demuxer=mp3 \
  --enable-demuxer=aac \
  --enable-decoder=aac \
  --enable-decoder=aac_latm \
  --enable-decoder=mp3 \
  --enable-decoder=mp3float \
  --enable-decoder=mp3on4 \
  --enable-decoder=mp3on4float \
  --enable-decoder=vorbis \
  --enable-decoder=opus \
  --enable-decoder=flac \
  --enable-decoder=pcm_s16le \
  --enable-decoder=pcm_s24le \
  --enable-decoder=pcm_s32le \
  --enable-decoder=pcm_f32le \
  --enable-decoder=pcm_f64le \
  --enable-encoder=pcm_s16le \
  --enable-muxer=wav \
  --enable-filter=aresample \
  --enable-filter=aformat \
  --enable-filter=highpass \
  --enable-filter=lowpass \
  --enable-filter=afftdn \
  --enable-filter=adeclick \
  --enable-filter=deesser \
  --enable-filter=dynaudnorm \
  --enable-parser=aac \
  --enable-parser=mp3 \
  --enable-parser=vorbis \
  --enable-parser=opus \
  --enable-parser=flac \
  --prefix=${PREFIX} \
  --pkg-config-flags="--static" \
  --extra-cflags="-I${PREFIX}/include" \
  --extra-ldflags="-static"
RUN --mount=type=cache,target=/root/.cache/ccache --mount=type=cache,target=/root/.cache/ffmpeg make -j"$(nproc)"
RUN make install
RUN strip ${PREFIX}/bin/ffmpeg || true
RUN rm -rf ${PREFIX}/share ${PREFIX}/include ${PREFIX}/lib

# --- Stage 2: Build Go server ---
FROM --platform=${BUILDPLATFORM} golang:1.25.3-alpine3.22 AS go-builder
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
WORKDIR /app
COPY main.go .
RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/go/pkg/mod \
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o /out/server main.go

# --- Stage 3: Runtime image ---
FROM --platform=${BUILDPLATFORM} scratch
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
ENV PATH=/usr/local/bin
COPY --from=ffmpeg-builder /opt/ffmpeg/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=go-builder /out/server /usr/local/bin/server
EXPOSE 8080
CMD ["/usr/local/bin/server"]

