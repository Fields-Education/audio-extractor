package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"slices"
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
	return runFFmpegWithTempInputArgs(r, nil, outputArgs)
}

func runFFmpegWithTempInputArgs(r io.Reader, inputArgs []string, outputArgs []string) ([]byte, error) {
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

	// Build ffmpeg args with file input instead of pipe. Segment seeks pass -ss
	// before -i so ffmpeg can seek against this real file instead of pipe:0.
	args := []string{
		"-hide_banner", "-loglevel", ffmpegLogLevel(),
	}
	args = append(args, inputArgs...)
	args = append(args, "-i", tmpFile.Name())
	args = append(args, outputArgs...)
	args = append(args, "pipe:1")

	// Add timeout to prevent hung ffmpeg processes
	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
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

func segmentInputArgs(startSec float64) []string {
	if startSec <= 0 {
		return nil
	}
	return []string{"-ss", fmt.Sprintf("%.3f", startSec)}
}

func segmentDurationArgs(durationSec float64) []string {
	return []string{"-t", fmt.Sprintf("%.3f", durationSec)}
}

const defaultMP3Bitrate = "128k"

// validMP3Bitrates lists the bitrates LAME supports for 16 kHz mono output
// (MPEG-2 Layer III). Callers pass them with a "k" suffix, matching ffmpeg's
// -b:a syntax.
var validMP3Bitrates = []string{
	"8k", "16k", "24k", "32k", "40k", "48k", "56k", "64k",
	"80k", "96k", "112k", "128k", "144k", "160k",
}

// audioContentTypes maps each audio output format to its response Content-Type.
var audioContentTypes = map[string]string{
	"wav":  "audio/wav",
	"mp3":  "audio/mpeg",
	"flac": "audio/flac",
}

// audioOutputArgs builds the ffmpeg output args for a 16 kHz mono audio format.
//
// Audio outputs drop video, subtitle and data streams so cover art (often PNG)
// and chapter text tracks in MP4/M4A/M4B inputs are not auto-mapped into
// muxers that accept attached pictures (MP3, FLAC); our minimal ffmpeg build
// has no PNG encoder. Chapters and global metadata are stripped too, and MP3
// output skips the ID3v2 tag, so an MP3 starts directly with audio frames and
// can be sliced by byte range.
func audioOutputArgs(format string, filterMask int, mp3Bitrate string) []string {
	var args []string

	if audioFilter := buildAudioFilter(filterMask); audioFilter != "" {
		args = append(args, "-af", audioFilter)
	}

	args = append(args,
		"-vn", "-sn", "-dn",
		"-map_chapters", "-1",
		"-map_metadata", "-1",
		"-ac", "1",
		"-ar", "16000",
	)

	switch format {
	case "wav":
		args = append(args, "-f", "wav", "-acodec", "pcm_s16le")
	case "mp3":
		args = append(args,
			"-f", "mp3",
			"-acodec", "libmp3lame",
			"-b:a", mp3Bitrate,
			"-id3v2_version", "0",
		)
	case "flac":
		// Pin 16-bit samples: float decoders (AAC, MP3, Opus) would otherwise
		// make the flac encoder pick s32 and write 24-bit output.
		args = append(args, "-f", "flac", "-acodec", "flac", "-sample_fmt", "s16", "-compression_level", "5")
	}

	return args
}

func convertAudio(r io.Reader, format string, filterMask int, mp3Bitrate string) ([]byte, error) {
	return runFFmpegWithTempInput(r, audioOutputArgs(format, filterMask, mp3Bitrate))
}

func convertAudioSegment(r io.Reader, format string, filterMask int, mp3Bitrate string, startSec, durationSec float64) ([]byte, error) {
	outputArgs := append(segmentDurationArgs(durationSec), audioOutputArgs(format, filterMask, mp3Bitrate)...)
	return runFFmpegWithTempInputArgs(r, segmentInputArgs(startSec), outputArgs)
}

func extractJpegPosterWithCleanup(r io.Reader) ([]byte, error) {
	outputArgs := []string{
		"-map", "0:v:0",
		"-frames:v", "1",
		"-an",
		"-vf", "blackframe=amount=0:threshold=32,metadata=select:key=lavfi.blackframe.pblack:value=98:function=less,scale=1280:-2",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "3",
	}

	data, err := runFFmpegWithTempInput(r, outputArgs)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("no non-black video frame found")
	}

	return data, nil
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

func parseSegmentSeconds(param string) (float64, error) {
	if param == "" {
		return 0, fmt.Errorf("missing value")
	}

	value, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return 0, err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("must be finite")
	}

	return value, nil
}

// parseMP3Bitrate validates the bitrate query param for MP3 output. An empty
// param means the default. Other formats ignore the param entirely, so a stray
// value never fails a WAV, FLAC or poster request.
func parseMP3Bitrate(format, param string) (string, error) {
	if format != "mp3" || param == "" {
		return defaultMP3Bitrate, nil
	}
	if !slices.Contains(validMP3Bitrates, param) {
		return "", fmt.Errorf("unsupported bitrate %q", param)
	}
	return param, nil
}

// writeBufferedResponse writes a fully buffered body with an explicit
// Content-Length so large outputs are not sent chunked; consumers such as R2
// uploads need a stream of known length.
func writeBufferedResponse(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		log.Printf("failed to write response: %v", err)
	}
}

var maxUploadSize int64 = 250 << 20 // 250MB default

const defaultTimeout = 5 * time.Minute

// ffmpegTimeout bounds each ffmpeg run. FFMPEG_TIMEOUT overrides it for long
// inputs such as multi-hour audiobooks.
var ffmpegTimeout = defaultTimeout

// durationFromEnv reads a positive Go duration (e.g. "45m") from the named
// environment variable. Unset returns def; an invalid value logs a warning and
// returns def.
func durationFromEnv(name string, def time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return def
	}

	d, err := time.ParseDuration(strings.TrimSpace(raw))
	if err == nil && d <= 0 {
		err = fmt.Errorf("must be positive")
	}
	if err != nil {
		log.Printf("warning: invalid %s '%s', using default %s: %v", name, raw, def, err)
		return def
	}
	return d
}

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

	mp3Bitrate, err := parseMP3Bitrate(format, r.URL.Query().Get("bitrate"))
	if err != nil {
		http.Error(w, "invalid bitrate", http.StatusBadRequest)
		return
	}

	var data []byte
	var contentType string

	switch format {
	case "wav", "mp3", "flac":
		data, err = convertAudio(r.Body, format, filterMask, mp3Bitrate)
		contentType = audioContentTypes[format]
	case "jpg", "jpeg", "poster":
		data, err = extractJpegPosterWithCleanup(r.Body)
		contentType = "image/jpeg"
	default:
		http.Error(w, fmt.Sprintf("unsupported format: %s", format), http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Printf("convert error for format %s: %v", format, err)
		http.Error(w, "conversion failed", http.StatusBadRequest)
		return
	}

	writeBufferedResponse(w, contentType, data)
}

func segmentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	defer func() { _ = r.Body.Close() }() // Explicitly ignore close error

	startSec, err := parseSegmentSeconds(r.URL.Query().Get("start_sec"))
	if err != nil || startSec < 0 {
		http.Error(w, "invalid start_sec", http.StatusBadRequest)
		return
	}

	durationSec, err := parseSegmentSeconds(r.URL.Query().Get("duration_sec"))
	if err != nil || durationSec <= 0 {
		http.Error(w, "invalid duration_sec", http.StatusBadRequest)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "mp3"
	}

	filterMask := parseFilterMask(r.URL.Query().Get("filters"))

	mp3Bitrate, err := parseMP3Bitrate(format, r.URL.Query().Get("bitrate"))
	if err != nil {
		http.Error(w, "invalid bitrate", http.StatusBadRequest)
		return
	}

	contentType, ok := audioContentTypes[format]
	if !ok {
		http.Error(w, fmt.Sprintf("unsupported format: %s", format), http.StatusBadRequest)
		return
	}

	data, err := convertAudioSegment(r.Body, format, filterMask, mp3Bitrate, startSec, durationSec)
	if err != nil {
		log.Printf("segment error for format %s: %v", format, err)
		http.Error(w, "segment conversion failed", http.StatusBadRequest)
		return
	}

	writeBufferedResponse(w, contentType, data)
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

	ffmpegTimeout = durationFromEnv("FFMPEG_TIMEOUT", defaultTimeout)
	httpReadTimeout := durationFromEnv("HTTP_READ_TIMEOUT", defaultTimeout)
	httpWriteTimeout := durationFromEnv("HTTP_WRITE_TIMEOUT", defaultTimeout)

	mux := http.NewServeMux()
	mux.HandleFunc("/convert", convertHandler)
	mux.HandleFunc("/segment", segmentHandler)
	mux.HandleFunc("/health", healthHandler)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       120 * time.Second,
	}
	if verbose {
		log.Printf("verbose logging enabled")
		log.Printf("max upload size: %d MB", maxUploadSize/(1<<20))
		log.Printf("timeouts: ffmpeg %s, http read %s, http write %s", ffmpegTimeout, httpReadTimeout, httpWriteTimeout)
	}
	log.Printf("audio-extractor %s listening on :%s", version, port)
	log.Fatal(srv.ListenAndServe())
}
