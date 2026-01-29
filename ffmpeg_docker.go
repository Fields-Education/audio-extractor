//go:build docker

package main

func init() {
	// In Docker mode, ffmpeg is expected to be in PATH
	ffmpegPath = "ffmpeg"
}
