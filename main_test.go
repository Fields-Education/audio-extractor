package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeFFmpegOutput is larger than net/http's pre-chunking buffer, so a
// response only carries Content-Length if the handler sets it explicitly.
var fakeFFmpegOutput = strings.Repeat("x", 8192)

// installFakeFFmpeg points ffmpegPath at a script that records its args and
// input size, checks the input is a real file, and prints fakeFFmpegOutput.
// With FFMPEG_SLEEP set it sleeps that many seconds instead, first writing its
// PID to FFMPEG_PID_FILE when that is set.
// The returned functions read the args and input size from the last invocation.
func installFakeFFmpeg(t *testing.T) (readArgs func() []string, readInputSize func() int) {
	t.Helper()

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
if [ -n "$FFMPEG_SLEEP" ]; then
	if [ -n "$FFMPEG_PID_FILE" ]; then
		echo $$ > "$FFMPEG_PID_FILE"
	fi
	exec sleep "$FFMPEG_SLEEP"
fi
head -c ` + strconv.Itoa(len(fakeFFmpegOutput)) + ` /dev/zero | tr '\0' x
`
	if err := os.WriteFile(fakeFFmpegPath, []byte(fakeFFmpeg), 0755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}

	ffmpegPath = fakeFFmpegPath
	t.Setenv("FFMPEG_ARGS_FILE", argsPath)
	t.Setenv("FFMPEG_INPUT_SIZE_FILE", inputSizePath)

	readArgs = func() []string {
		t.Helper()
		data, err := os.ReadFile(argsPath)
		if err != nil {
			t.Fatalf("read fake ffmpeg args: %v", err)
		}
		return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	}
	readInputSize = func() int {
		t.Helper()
		data, err := os.ReadFile(inputSizePath)
		if err != nil {
			t.Fatalf("read fake ffmpeg input size: %v", err)
		}
		size, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			t.Fatalf("parse fake ffmpeg input size: %v", err)
		}
		return size
	}
	return readArgs, readInputSize
}

func TestSegmentConversionUsesSeekableInputFile(t *testing.T) {
	readArgs, readInputSize := installFakeFFmpeg(t)

	source := []byte("source media bytes")
	data, err := convertAudioSegment(context.Background(), bytes.NewReader(source), "mp3", 0, defaultMP3Bitrate, 12.3456, 6.789)
	if err != nil {
		t.Fatalf("segment conversion failed: %v", err)
	}
	if string(data) != fakeFFmpegOutput {
		t.Fatalf("unexpected fake ffmpeg output %q", data)
	}

	args := readArgs()

	if slices.Index(args, "pipe:0") != -1 {
		t.Fatalf("segment input must not use pipe:0; args=%v", args)
	}

	ssIndex := slices.Index(args, "-ss")
	inputIndex := slices.Index(args, "-i")
	durationIndex := slices.Index(args, "-t")
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

	if inputSize := readInputSize(); inputSize != len(source) {
		t.Fatalf("ffmpeg input size = %d, want %d", inputSize, len(source))
	}
}

func TestAudioOutputsDropNonAudioStreamsAndMetadata(t *testing.T) {
	readArgs, _ := installFakeFFmpeg(t)

	for _, target := range []string{
		"/convert?format=wav",
		"/convert?format=mp3",
		"/convert?format=flac",
		"/segment?start_sec=1&duration_sec=2&format=wav",
		"/segment?start_sec=1&duration_sec=2&format=mp3",
		"/segment?start_sec=1&duration_sec=2&format=flac",
	} {
		t.Run(target, func(t *testing.T) {
			rec := serve(t, target)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
			}

			args := readArgs()
			inputIndex := slices.Index(args, "-i")
			flags := []string{"-vn", "-sn", "-dn", "-map_chapters", "-map_metadata"}
			if strings.Contains(target, "format=mp3") {
				flags = append(flags, "-id3v2_version")
			}
			if strings.Contains(target, "format=flac") {
				flags = append(flags, "-sample_fmt")
			}
			for _, flag := range flags {
				// Output options must follow -i; before it they would apply to the input.
				if i := slices.Index(args, flag); i <= inputIndex {
					t.Errorf("%s at %d, want after -i at %d; args=%v", flag, i, inputIndex, args)
				}
			}
			for _, flag := range []string{"-map_chapters", "-map_metadata"} {
				if got := argValue(args, flag); got != "-1" {
					t.Errorf("%s = %q, want -1; args=%v", flag, got, args)
				}
			}
			if strings.Contains(target, "format=mp3") {
				if got := argValue(args, "-id3v2_version"); got != "0" {
					t.Errorf("-id3v2_version = %q, want 0; args=%v", got, args)
				}
			}
			if strings.Contains(target, "format=flac") {
				if got := argValue(args, "-sample_fmt"); got != "s16" {
					t.Errorf("-sample_fmt = %q, want s16; args=%v", got, args)
				}
			}
		})
	}
}

func TestPosterOutputKeepsVideoStream(t *testing.T) {
	readArgs, _ := installFakeFFmpeg(t)

	rec := serve(t, "/convert?format=poster")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}

	args := readArgs()
	if got := argValue(args, "-map"); got != "0:v:0" {
		t.Fatalf("-map = %q, want 0:v:0; args=%v", got, args)
	}
	for _, flag := range []string{"-vn", "-sn", "-dn", "-map_chapters", "-map_metadata"} {
		if slices.Index(args, flag) != -1 {
			t.Errorf("poster args must not include %s; args=%v", flag, args)
		}
	}
}

func TestMP3Bitrate(t *testing.T) {
	readArgs, _ := installFakeFFmpeg(t)

	for _, endpoint := range []string{"/convert?format=mp3", "/segment?start_sec=0&duration_sec=5&format=mp3"} {
		t.Run(endpoint+" default", func(t *testing.T) {
			rec := serve(t, endpoint)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
			}
			if got := argValue(readArgs(), "-b:a"); got != "128k" {
				t.Fatalf("-b:a = %q, want 128k", got)
			}
		})

		for _, bitrate := range []string{"8k", "32k", "160k"} {
			t.Run(endpoint+" valid "+bitrate, func(t *testing.T) {
				rec := serve(t, endpoint+"&bitrate="+bitrate)
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
				}
				if got := argValue(readArgs(), "-b:a"); got != bitrate {
					t.Fatalf("-b:a = %q, want %s", got, bitrate)
				}
			})
		}

		t.Run(endpoint+" empty uses default", func(t *testing.T) {
			rec := serve(t, endpoint+"&bitrate=")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
			}
			if got := argValue(readArgs(), "-b:a"); got != "128k" {
				t.Fatalf("-b:a = %q, want 128k", got)
			}
		})

		for _, invalid := range []string{"32", "32000", "33k", "192k", "0k", "-32k", "32K"} {
			t.Run(endpoint+" invalid "+invalid, func(t *testing.T) {
				rec := serve(t, endpoint+"&bitrate="+invalid)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", rec.Code)
				}
			})
		}
	}
}

func TestBitrateIgnoredForNonMP3Formats(t *testing.T) {
	installFakeFFmpeg(t)

	for _, target := range []string{
		"/convert?bitrate=bogus",
		"/convert?format=flac&bitrate=bogus",
		"/convert?format=poster&bitrate=bogus",
		"/segment?start_sec=0&duration_sec=5&format=wav&bitrate=bogus",
	} {
		t.Run(target, func(t *testing.T) {
			if rec := serve(t, target); rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestDurationFromEnv(t *testing.T) {
	const name = "AUDIO_EXTRACTOR_TEST_TIMEOUT"
	def := 5 * time.Minute

	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"", def},
		{"45m", 45 * time.Minute},
		{" 1h30m ", 90 * time.Minute},
		{"90s", 90 * time.Second},
		{"45", def},
		{"forever", def},
		{"0s", def},
		{"-1m", def},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv(name, tc.value)
			if got := durationFromEnv(name, def); got != tc.want {
				t.Fatalf("durationFromEnv(%q) = %s, want %s", tc.value, got, tc.want)
			}
		})
	}
}

func TestFFmpegTimeoutStopsLongRuns(t *testing.T) {
	installFakeFFmpeg(t)
	t.Setenv("FFMPEG_SLEEP", "30")

	original := ffmpegTimeout
	ffmpegTimeout = 200 * time.Millisecond
	t.Cleanup(func() { ffmpegTimeout = original })

	start := time.Now()
	if _, err := convertAudio(context.Background(), strings.NewReader("input"), "mp3", 0, defaultMP3Bitrate); err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("ffmpeg ran for %s, want it stopped near %s", elapsed, ffmpegTimeout)
	}
}

func TestClientDisconnectStopsFFmpeg(t *testing.T) {
	installFakeFFmpeg(t)
	// Short enough that a regression fails fast; srv.Close waits for the handler.
	t.Setenv("FFMPEG_SLEEP", "15")

	for _, target := range []string{
		"/convert?format=mp3",
		"/convert?format=poster",
		"/segment?start_sec=0&duration_sec=5",
	} {
		t.Run(target, func(t *testing.T) {
			pidPath := filepath.Join(t.TempDir(), "pid")
			t.Setenv("FFMPEG_PID_FILE", pidPath)

			handlerDone := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				if r.URL.Path == "/segment" {
					segmentHandler(w, r)
				} else {
					convertHandler(w, r)
				}
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+target, strings.NewReader("input"))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			clientDone := make(chan error, 1)
			go func() {
				resp, err := http.DefaultClient.Do(req)
				if err == nil {
					_ = resp.Body.Close()
				}
				clientDone <- err
			}()

			pid := waitForPID(t, pidPath)
			cancel() // Disconnect while ffmpeg is still running.
			if err := <-clientDone; !errors.Is(err, context.Canceled) {
				t.Fatalf("client error = %v, want context.Canceled", err)
			}

			select {
			case <-handlerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("handler still running 5s after the client disconnected")
			}
			// The handler returns only after exec reaps ffmpeg, so the PID must be gone.
			if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
				t.Fatalf("ffmpeg pid %d still exists after disconnect (kill err %v)", pid, err)
			}
		})
	}
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && len(bytes.TrimSpace(data)) > 0 {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatalf("parse pid: %v", err)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fake ffmpeg never started")
	return 0
}

func TestSuccessfulResponsesSetContentLength(t *testing.T) {
	installFakeFFmpeg(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/convert", convertHandler)
	mux.HandleFunc("/segment", segmentHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	for _, target := range []string{
		"/convert?format=mp3",
		"/convert?format=poster",
		"/segment?start_sec=0&duration_sec=5",
	} {
		t.Run(target, func(t *testing.T) {
			resp, err := http.Post(srv.URL+target, "application/octet-stream", strings.NewReader("input"))
			if err != nil {
				t.Fatalf("post: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, body %q", resp.StatusCode, body)
			}
			if len(resp.TransferEncoding) != 0 {
				t.Errorf("Transfer-Encoding = %v, want none", resp.TransferEncoding)
			}
			if resp.ContentLength != int64(len(body)) || len(body) != len(fakeFFmpegOutput) {
				t.Errorf("Content-Length = %d, body length = %d", resp.ContentLength, len(body))
			}
		})
	}
}

func serve(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader("input"))
	rec := httptest.NewRecorder()
	if strings.HasPrefix(target, "/segment") {
		segmentHandler(rec, req)
	} else {
		convertHandler(rec, req)
	}
	return rec
}

// argValue returns the value following flag in args, or "" if absent.
func argValue(args []string, flag string) string {
	i := slices.Index(args, flag)
	if i == -1 || i+1 >= len(args) {
		return ""
	}
	return args[i+1]
}
