package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ffmpegPath is set by init() in either ffmpeg_embedded.go or ffmpeg_docker.go
var ffmpegPath string

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

func convertToWavPcm16(r io.Reader) ([]byte, error) {
	return convertToWavPcm16WithCleanup(r, 0)
}

func convertToWavPcm16WithCleanup(r io.Reader, filterMask int) ([]byte, error) {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
	}

	audioFilter := buildAudioFilter(filterMask)
	if audioFilter != "" {
		args = append(args, "-af", audioFilter)
	}

	args = append(args,
		"-ac", "1",
		"-ar", "16000",
		"-f", "wav",
		"-acodec", "pcm_s16le",
		"pipe:1",
	)

	cmd := exec.Command(ffmpegPath, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer stdin.Close()
		io.Copy(stdin, r)
	}()
	if err := cmd.Run(); err != nil {
		log.Printf("ffmpeg stderr: %s", stderr.String())
		return nil, fmt.Errorf("%v: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

func convertToMp3(r io.Reader) ([]byte, error) {
	return convertToMp3WithCleanup(r, 0)
}

func convertToMp3WithCleanup(r io.Reader, filterMask int) ([]byte, error) {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
	}

	audioFilter := buildAudioFilter(filterMask)
	if audioFilter != "" {
		args = append(args, "-af", audioFilter)
	}

	args = append(args,
		"-ac", "1",
		"-ar", "16000",
		"-f", "mp3",
		"-acodec", "libmp3lame",
		"-b:a", "128k",
		"pipe:1",
	)

	cmd := exec.Command(ffmpegPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer stdin.Close()
		io.Copy(stdin, r)
	}()
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func convertToFlac(r io.Reader) ([]byte, error) {
	return convertToFlacWithCleanup(r, 0)
}

func convertToFlacWithCleanup(r io.Reader, filterMask int) ([]byte, error) {
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
	}

	audioFilter := buildAudioFilter(filterMask)
	if audioFilter != "" {
		args = append(args, "-af", audioFilter)
	}

	args = append(args,
		"-ac", "1",
		"-ar", "16000",
		"-f", "flac",
		"-acodec", "flac",
		"-compression_level", "5",
		"pipe:1",
	)

	cmd := exec.Command(ffmpegPath, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	go func() {
		defer stdin.Close()
		io.Copy(stdin, r)
	}()
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func parseFilterMask(filterParam string) int {
	if filterParam == "" {
		return 0
	}

	if filterParam == "true" || filterParam == "1" || filterParam == "all" {
		return FilterAll
	}

	mask, err := strconv.Atoi(filterParam)
	if err != nil {
		return 0
	}

	return mask
}

func convertHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

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
	w.Write(data)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/convert", convertHandler)
	mux.HandleFunc("/health", healthHandler)
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	log.Printf("ffmpeg audio extractor v2 listening on :8080")
	log.Fatal(srv.ListenAndServe())
}
