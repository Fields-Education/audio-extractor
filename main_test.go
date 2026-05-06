package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var testPosterJPEG = []byte{0xff, 0xd8, 0xff, 0xd9}

func TestPosterContentTypeSupportsWebMVP8Opus(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{
			name:        "unquoted codecs parameter",
			contentType: "video/webm;codecs=vp8,opus",
			want:        true,
		},
		{
			name:        "quoted codecs parameter",
			contentType: `video/webm; codecs="vp8,opus"`,
			want:        true,
		},
		{
			name:        "octet stream fallback",
			contentType: "application/octet-stream",
			want:        true,
		},
		{
			name:        "empty content type",
			contentType: "",
			want:        true,
		},
		{
			name:        "non-video webm",
			contentType: "audio/webm",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSupportedPosterContentType(tt.contentType)
			if got != tt.want {
				t.Fatalf("isSupportedPosterContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestPosterHandlerAcceptsWebMVP8OpusContentType(t *testing.T) {
	fakeFFmpegPath, argsPath := writeFakeFFmpeg(t)
	t.Setenv("FFMPEG_ARGS_FILE", argsPath)

	previousFFmpegPath := ffmpegPath
	ffmpegPath = fakeFFmpegPath
	t.Cleanup(func() {
		ffmpegPath = previousFFmpegPath
	})

	req := httptest.NewRequest(http.MethodPost, "/poster", strings.NewReader("fake webm"))
	req.Header.Set("Content-Type", "video/webm;codecs=vp8,opus")
	rr := httptest.NewRecorder()

	posterHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	if !bytes.Equal(rr.Body.Bytes(), testPosterJPEG) {
		t.Fatalf("body = %v, want %v", rr.Body.Bytes(), testPosterJPEG)
	}

	argsBytes, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("failed to read ffmpeg args: %v", err)
	}
	args := strings.Split(strings.TrimSpace(string(argsBytes)), "\n")
	assertContainsInOrder(t, args, []string{
		"-map", "0:v:0",
		"-frames:v", "1",
		"-an",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	})
}

func writeFakeFFmpeg(t *testing.T) (string, string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("shell fake is not supported on Windows")
	}

	dir := t.TempDir()
	ffmpegPath := filepath.Join(dir, "ffmpeg")
	argsPath := filepath.Join(dir, "args")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$@" > "$FFMPEG_ARGS_FILE"
printf '\377\330\377\331'
`
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o700); err != nil {
		t.Fatalf("failed to write fake ffmpeg: %v", err)
	}

	return ffmpegPath, argsPath
}

func assertContainsInOrder(t *testing.T, got []string, want []string) {
	t.Helper()

	offset := 0
	for _, wantArg := range want {
		found := false
		for offset < len(got) {
			if got[offset] == wantArg {
				found = true
				offset++
				break
			}
			offset++
		}
		if !found {
			t.Fatalf("ffmpeg args %v did not contain %q in order", got, wantArg)
		}
	}
}
