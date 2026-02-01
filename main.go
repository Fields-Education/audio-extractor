package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Version information - injected at build time via ldflags
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// ffmpegPath is set by init() in either ffmpeg_embedded.go or ffmpeg_docker.go
var ffmpegPath string

// verbose controls whether ffmpeg output is logged
var verbose bool

const (
	FilterHighpass           = 1 << 0 // 1   - Remove low-frequency rumble
	FilterLowpass            = 1 << 1 // 2   - Remove high-frequency noise
	FilterDenoiser           = 1 << 2 // 4   - FFT-based noise reduction
	FilterDeclick            = 1 << 3 // 8   - Remove clicks and pops
	FilterDeesser            = 1 << 4 // 16  - Reduce harsh sibilance
	FilterNormalize          = 1 << 5 // 32  - Loudness normalization
	FilterDenoiserSpeechMode = 1 << 6 // 64  - Enable speech mode for denoiser (requires FilterDenoiser)
	FilterAll                = FilterHighpass | FilterLowpass | FilterDenoiser | FilterDeclick | FilterDeesser | FilterNormalize
)

func buildAudioFilter(filterMask int) string {
	if filterMask == 0 {
		return ""
	}

	var filters []string

	if filterMask&FilterHighpass != 0 {
		filters = append(filters, "highpass=f=75:p=1")
	}

	if filterMask&FilterLowpass != 0 {
		filters = append(filters, "lowpass=f=7500:p=1")
	}

	if filterMask&FilterDenoiser != 0 {
		noiseType := "w"
		if filterMask&FilterDenoiserSpeechMode != 0 {
			noiseType = "s"
		}
		filters = append(filters, fmt.Sprintf("afftdn=nf=-25:nt=%s", noiseType))
	}

	if filterMask&FilterDeclick != 0 {
		filters = append(filters, "adeclick=t=2:w=10")
	}

	if filterMask&FilterDeesser != 0 {
		filters = append(filters, "deesser")
	}

	if filterMask&FilterNormalize != 0 {
		filters = append(filters, "dynaudnorm")
	}

	return strings.Join(filters, ",")
}

func ffmpegLogLevel() string {
	if verbose {
		return "info"
	}
	return "error"
}

// runFFmpegWithTempInput writes the input to a temp file and runs ffmpeg.
// This is required for formats like MP4/MOV that need seeking to read metadata.
func runFFmpegWithTempInput(r io.Reader, outputArgs []string) ([]byte, error) {
	// Create temp file for input
	tmpFile, err := os.CreateTemp("", "audio-input-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }() // Best-effort cleanup

	// Write input to temp file
	if _, err := io.Copy(tmpFile, r); err != nil {
		_ = tmpFile.Close() // Explicitly ignore close error in error path
		return nil, fmt.Errorf("failed to write input: %w", err)
	}
	_ = tmpFile.Close() // Close before ffmpeg reads it, ignore error

	// Build ffmpeg args with file input instead of pipe
	args := []string{
		"-hide_banner", "-loglevel", ffmpegLogLevel(),
		"-i", tmpFile.Name(),
	}
	args = append(args, outputArgs...)
	args = append(args, "pipe:1")

	// Add timeout to prevent hung ffmpeg processes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("ffmpeg stderr: %s", stderr.String())
		return nil, fmt.Errorf("%v: %s", err, stderr.String())
	}
	if verbose && stderr.Len() > 0 {
		log.Printf("ffmpeg output: %s", stderr.String())
	}
	return stdout.Bytes(), nil
}

func convertToWavPcm16WithCleanup(r io.Reader, filterMask int) ([]byte, error) {
	var outputArgs []string

	audioFilter := buildAudioFilter(filterMask)
	if audioFilter != "" {
		outputArgs = append(outputArgs, "-af", audioFilter)
	}

	outputArgs = append(outputArgs,
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		"-acodec", "pcm_s16le",
	)

	return runFFmpegWithTempInput(r, outputArgs)
}

func convertToMp3WithCleanup(r io.Reader, filterMask int) ([]byte, error) {
	var outputArgs []string

	audioFilter := buildAudioFilter(filterMask)
	if audioFilter != "" {
		outputArgs = append(outputArgs, "-af", audioFilter)
	}

	outputArgs = append(outputArgs,
		"-ac", "1",
		"-ar", "16000",
		"-f", "mp3",
		"-acodec", "libmp3lame",
		"-b:a", "128k",
	)

	return runFFmpegWithTempInput(r, outputArgs)
}

func convertToFlacWithCleanup(r io.Reader, filterMask int) ([]byte, error) {
	var outputArgs []string

	audioFilter := buildAudioFilter(filterMask)
	if audioFilter != "" {
		outputArgs = append(outputArgs, "-af", audioFilter)
	}

	outputArgs = append(outputArgs,
		"-ac", "1",
		"-ar", "16000",
		"-f", "flac",
		"-acodec", "flac",
		"-compression_level", "5",
	)

	return runFFmpegWithTempInput(r, outputArgs)
}

func parseFilterMask(filterParam string) int {
	if filterParam == "" {
		return 0
	}

	if filterParam == "true" || filterParam == "all" {
		return FilterAll
	}

	mask, err := strconv.Atoi(filterParam)
	if err != nil {
		return 0
	}

	// Mask to valid filter bits to prevent overflow and invalid values
	return mask & (FilterAll | FilterDenoiserSpeechMode)
}

var maxUploadSize int64 = 250 << 20 // 250MB default

// parseByteSize parses strings like "250MB", "1GB", "500" (bytes)
func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1 << 30
		s = strings.TrimSuffix(s, "GB")
	case strings.HasSuffix(s, "MB"):
		multiplier = 1 << 20
		s = strings.TrimSuffix(s, "MB")
	case strings.HasSuffix(s, "KB"):
		multiplier = 1 << 10
		s = strings.TrimSuffix(s, "KB")
	}

	s = strings.TrimSpace(s)
	value, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size value: %w", err)
	}

	// Reject negative values
	if value <= 0 {
		return 0, fmt.Errorf("size must be positive")
	}

	// Check for overflow before multiplication
	if multiplier > 1 && value > (1<<63-1)/multiplier {
		return 0, fmt.Errorf("size value too large: overflow")
	}

	return value * multiplier, nil
}

func convertHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	defer func() { _ = r.Body.Close() }() // Explicitly ignore close error

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "wav"
	}

	filterMask := parseFilterMask(r.URL.Query().Get("filters"))

	var data []byte
	var err error
	var contentType string

	switch format {
	case "wav":
		data, err = convertToWavPcm16WithCleanup(r.Body, filterMask)
		contentType = "audio/wav"
	case "mp3":
		data, err = convertToMp3WithCleanup(r.Body, filterMask)
		contentType = "audio/mpeg"
	case "flac":
		data, err = convertToFlacWithCleanup(r.Body, filterMask)
		contentType = "audio/flac"
	default:
		http.Error(w, fmt.Sprintf("unsupported format: %s", format), http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Printf("convert error for format %s: %v", format, err)
		http.Error(w, "conversion failed", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("failed to write health response: %v", err)
		}
	}
}

func main() {
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose logging of ffmpeg output")
	flag.BoolVar(&verbose, "v", false, "Enable verbose logging of ffmpeg output (shorthand)")
	flag.Parse()

	if *showVersion {
		fmt.Printf("audio-extractor %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
		os.Exit(0)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Also allow verbose via environment variable
	if os.Getenv("VERBOSE") == "true" || os.Getenv("VERBOSE") == "1" {
		verbose = true
	}

	// Parse max upload size from env var
	if uploadSize := os.Getenv("MAX_UPLOAD_SIZE"); uploadSize != "" {
		size, err := parseByteSize(uploadSize)
		if err != nil {
			log.Printf("warning: invalid MAX_UPLOAD_SIZE '%s', using default 250MB: %v", uploadSize, err)
		} else if size > 0 {
			maxUploadSize = size
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/convert", convertHandler)
	mux.HandleFunc("/health", healthHandler)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}
	if verbose {
		log.Printf("verbose logging enabled")
		log.Printf("max upload size: %d MB", maxUploadSize/(1<<20))
	}
	log.Printf("audio-extractor %s listening on :%s", version, port)
	log.Fatal(srv.ListenAndServe())
}
