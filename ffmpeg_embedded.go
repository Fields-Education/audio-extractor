//go:build !docker

package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
)

//go:embed embed/ffmpeg/*
var embeddedFFmpeg embed.FS

func init() {
	var err error
	ffmpegPath, err = extractFFmpeg()
	if err != nil {
		log.Fatalf("failed to extract ffmpeg: %v", err)
	}
}

func extractFFmpeg() (string, error) {
	// Determine the correct embedded binary for this platform
	binaryName := fmt.Sprintf("ffmpeg_%s_%s", runtime.GOOS, runtime.GOARCH)
	embeddedPath := fmt.Sprintf("embed/ffmpeg/%s", binaryName)

	data, err := embeddedFFmpeg.ReadFile(embeddedPath)
	if err != nil {
		return "", fmt.Errorf("no embedded ffmpeg for %s/%s: %w", runtime.GOOS, runtime.GOARCH, err)
	}

	// Create cache directory
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	extractDir := filepath.Join(cacheDir, "audio-extractor")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache dir: %w", err)
	}

	// Use hash of binary content to detect updates
	hash := sha256.Sum256(data)
	hashStr := hex.EncodeToString(hash[:8]) // First 8 bytes is enough
	extractPath := filepath.Join(extractDir, fmt.Sprintf("ffmpeg-%s", hashStr))

	// Check if already extracted
	if info, err := os.Stat(extractPath); err == nil && info.Mode().Perm()&0111 != 0 {
		log.Printf("using cached ffmpeg at %s", extractPath)
		return extractPath, nil
	}

	// Extract the binary
	if err := os.WriteFile(extractPath, data, 0755); err != nil {
		return "", fmt.Errorf("failed to write ffmpeg: %w", err)
	}

	log.Printf("extracted ffmpeg to %s", extractPath)
	return extractPath, nil
}
