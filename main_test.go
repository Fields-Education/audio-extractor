package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestSegmentConversionUsesSeekableInputFile(t *testing.T) {
	originalFFmpegPath := ffmpegPath
	t.Cleanup(func() { ffmpegPath = originalFFmpegPath })

	tmpDir := t.TempDir()
	fakeFFmpegPath := filepath.Join(tmpDir, "ffmpeg")
	argsPath := filepath.Join(tmpDir, "args")
	inputSizePath := filepath.Join(tmpDir, "input-size")

	fakeFFmpeg := `#!/bin/sh
printf '%s\n' "$@" > "$FFMPEG_ARGS_FILE"
input=''
prev=''
for arg in "$@"; do
	if [ "$prev" = "-i" ]; then
		input="$arg"
	fi
	prev="$arg"
done
if [ ! -f "$input" ]; then
	echo "input is not a file: $input" >&2
	exit 17
fi
wc -c < "$input" > "$FFMPEG_INPUT_SIZE_FILE"
printf 'ok'
`
	if err := os.WriteFile(fakeFFmpegPath, []byte(fakeFFmpeg), 0755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	ffmpegPath = fakeFFmpegPath
	t.Setenv("FFMPEG_ARGS_FILE", argsPath)
	t.Setenv("FFMPEG_INPUT_SIZE_FILE", inputSizePath)

	source := []byte("source media bytes")
	data, err := convertToMp3SegmentWithCleanup(bytes.NewReader(source), 0, 12.3456, 6.789)
	if err != nil {
		t.Fatalf("segment conversion failed: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("unexpected fake ffmpeg output %q", data)
	}

	argsData, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake ffmpeg args: %v", err)
	}
	args := strings.Split(strings.TrimSuffix(string(argsData), "\n"), "\n")

	if indexOf(args, "pipe:0") != -1 {
		t.Fatalf("segment input must not use pipe:0; args=%v", args)
	}

	ssIndex := indexOf(args, "-ss")
	inputIndex := indexOf(args, "-i")
	durationIndex := indexOf(args, "-t")
	if ssIndex == -1 || inputIndex == -1 || durationIndex == -1 {
		t.Fatalf("missing expected segment args; args=%v", args)
	}
	if ssIndex > inputIndex {
		t.Fatalf("-ss must be before -i for input seeking; args=%v", args)
	}
	if got := args[ssIndex+1]; got != "12.346" {
		t.Fatalf("unexpected -ss value %q", got)
	}
	if got := args[durationIndex+1]; got != "6.789" {
		t.Fatalf("unexpected -t value %q", got)
	}
	if inputPath := args[inputIndex+1]; inputPath == "pipe:0" || !strings.Contains(filepath.Base(inputPath), "audio-input-") {
		t.Fatalf("expected temp file input, got %q", inputPath)
	}

	inputSizeData, err := os.ReadFile(inputSizePath)
	if err != nil {
		t.Fatalf("read fake ffmpeg input size: %v", err)
	}
	inputSize, err := strconv.Atoi(strings.TrimSpace(string(inputSizeData)))
	if err != nil {
		t.Fatalf("parse fake ffmpeg input size: %v", err)
	}
	if inputSize != len(source) {
		t.Fatalf("ffmpeg input size = %d, want %d", inputSize, len(source))
	}
}

func indexOf(values []string, target string) int {
	for i, value := range values {
		if value == target {
			return i
		}
	}
	return -1
}
