# syntax=docker/dockerfile:1.6

# --- Stage 1: Build ffmpeg (audio-only minimal) ---
# Must build on target platform since FFmpeg can't easily cross-compile
FROM --platform=${TARGETPLATFORM} alpine:latest AS ffmpeg-builder
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
RUN apk add --no-cache \
  ca-certificates curl git build-base pkgconf nasm yasm automake autoconf libtool zlib-dev zlib-static binutils ccache \
  lame-dev

ARG FFMPEG_VERSION=8.0
ENV PREFIX=/opt/ffmpeg
ENV CCACHE_DIR=/root/.cache/ccache
# Put ccache symlinks first in PATH so gcc/g++ calls go through ccache
ENV PATH=/usr/lib/ccache/bin:$PATH
WORKDIR /build
RUN curl -L -o ffmpeg.tar.xz https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz
# Extract to fixed path so ccache hits work across version bumps
RUN mkdir -p /build/ffmpeg && tar xf ffmpeg.tar.xz --strip-components=1 -C /build/ffmpeg
WORKDIR /build/ffmpeg
# Copy shared configure script
COPY scripts/ffmpeg-configure.sh /build/ffmpeg-configure.sh
RUN --mount=type=cache,target=/root/.cache/ccache /build/ffmpeg-configure.sh \
  --prefix=${PREFIX} \
  --pkg-config-flags="--static" \
  --extra-cflags="-I${PREFIX}/include" \
  --extra-ldflags="-static"
# Show ccache stats before build
RUN --mount=type=cache,target=/root/.cache/ccache ccache -s || true
RUN --mount=type=cache,target=/root/.cache/ccache make -j"$(nproc)"
# Show ccache stats after build to see hit rate
RUN --mount=type=cache,target=/root/.cache/ccache ccache -s
RUN make install
RUN strip ${PREFIX}/bin/ffmpeg || true
RUN rm -rf ${PREFIX}/share ${PREFIX}/include ${PREFIX}/lib

# --- Stage 2: Build Go server ---
FROM --platform=${BUILDPLATFORM} golang:1.25.6-alpine3.22 AS go-builder
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
WORKDIR /app
COPY go.mod .
COPY main.go ffmpeg_docker.go ./
RUN --mount=type=cache,target=/root/.cache/go-build \
  --mount=type=cache,target=/go/pkg/mod \
  CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -tags docker -trimpath -ldflags='-s -w' -o /out/server .

# --- Stage 3: Runtime image ---
FROM --platform=${TARGETPLATFORM} scratch
ARG TARGETPLATFORM
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
ENV PATH=/usr/local/bin
COPY --from=ffmpeg-builder /opt/ffmpeg/bin/ffmpeg /usr/local/bin/ffmpeg
COPY --from=go-builder /out/server /usr/local/bin/server
EXPOSE 8080
CMD ["/usr/local/bin/server"]

