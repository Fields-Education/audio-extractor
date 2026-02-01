# Audio Extractor Service

Audio conversion optimized for Dual Language Immersion classroom recordings with adaptive noise reduction for Chinese language learning.

## Quick Start

```bash
# Convert an audio file to WAV format with no filters (default)
curl -X POST http://localhost:8080/convert?format=wav \
  --data-binary @input.mp3 \
  --output output.wav

# Convert with ClassroomChromebook preset (111) - recommended for classroom recordings
curl -X POST "http://localhost:8080/convert?format=wav&filters=111" \
  --data-binary @input.mp3 \
  --output output.wav

# Convert to MP3 with basic cleanup filters (7)
curl -X POST "http://localhost:8080/convert?format=mp3&filters=7" \
  --data-binary @input.mp3 \
  --output output.mp3
```

## API

### POST /convert

**Query Parameters:**

- `format` - Output format: `wav` (default), `mp3`, or `flac`
- `filters` - Filter bitmask or keyword: `all`/`true` enables all filters (see Filter Bitmask below)

**Output Specs:**

- Sample rate: 16000 Hz
- Channels: Mono
- Bit depth: 16-bit (WAV/FLAC) or 128kbps (MP3)

### GET /health

Health check endpoint - returns `200 OK` with body `ok`

## Filter Bitmask

Combine filters using bitwise OR (addition):

| Bit | Value | Filter      | Description                                           |
| --- | ----- | ----------- | ----------------------------------------------------- |
| 0   | 1     | Highpass    | Remove frequencies < 75Hz (rumble, AC hum)            |
| 1   | 2     | Lowpass     | Remove frequencies > 7.5kHz (hiss, interference)      |
| 2   | 4     | Denoiser    | FFT-based adaptive noise reduction                    |
| 3   | 8     | Declick     | Remove clicks, pops, mouth sounds                     |
| 4   | 16    | Deesser     | Reduce harsh sibilance (s, sh, ch, t)                 |
| 5   | 32    | Normalize   | Dynamic loudness normalization                        |
| 6   | 64    | Speech Mode | Enable speech detection for denoiser (requires bit 2) |

### Common Presets

| Value         | Filters                  | Use Case               | Processing Time |
| ------------- | ------------------------ | ---------------------- | --------------- |
| `0`           | None                     | No processing          | ~100-200ms      |
| `7`           | HPF+LPF+Denoise          | Basic cleanup          | ~500ms-1s       |
| `36`          | Denoise+Normalize        | Gentle cleanup         | ~300-700ms      |
| `39`          | HPF+LPF+Denoise+Norm     | Balanced (recommended) | ~700ms-1.5s     |
| `63` or `all` | All (except speech mode) | Maximum cleanup        | ~1-3s           |

## Recommended Presets

Optimized for Dual Language Immersion (Chinese) classroom scenarios with fragmented speech, background students, and varying recording quality.

### ClassroomChromebook (111) - **Recommended for Classroom**

**Value:** `111`

```bash
# Use with curl
curl -X POST "http://localhost:8080/convert?format=wav&filters=111" \
  --data-binary @input.mp3 \
  --output output.wav
```

**Best for:** Chromebook recordings in classroom (most common scenario)

**Features:**

- Adaptive speech mode denoiser (learns during ums/ahs and pauses)
- Removes background students talking
- Handles room echo and HVAC noise
- Removes desk taps, chair scraping
- Balances volume levels

**Processing:** ~1.5-2.5s per 60s of audio

**Use when:**

- ✅ Recording on Chromebook in classroom
- ✅ Multiple students in background
- ✅ Student speech has pauses, ums, ahs
- ✅ Room has echo or ambient noise

### ClassroomSafe (47) - Fallback

```bash
# Use filter value 47 (no speech mode)
curl -X POST "http://localhost:8080/convert?format=wav&filters=47" \
  --data-binary @input.mp3 \
  --output output.wav
```

**Best for:** Same as ClassroomChromebook but more predictable processing

**Difference:** Uses white noise mode instead of speech mode

- More consistent processing
- Fewer artifacts during speech transitions
- Better for very continuous speech

**Processing:** ~1-2s per 60s of audio

**Use when:**

- ⚠️ Speech mode causes weird artifacts
- ⚠️ Student speaks continuously without pauses

### PhoneMobile (39)

```bash
# Use filter value 39 for phone recordings
curl -X POST "http://localhost:8080/convert?format=wav&filters=39" \
  --data-binary @input.mp3 \
  --output output.wav
```

**Best for:** iPhone/Android recordings

**Features:**

- Removes hand-holding rumble and movement noise
- Handles phone codec compression artifacts
- Removes wind noise if outdoors

**Processing:** ~700ms-1.5s per 60s of audio

### HomeQuiet (36)

```bash
# Use filter value 36 for minimal processing
curl -X POST "http://localhost:8080/convert?format=wav&filters=36" \
  --data-binary @input.mp3 \
  --output output.wav
```

**Best for:** Quiet home recordings (best quality scenario)

**Features:**

- Minimal processing preserves voice quality
- Gentle noise reduction
- Volume balancing

**Processing:** ~300-700ms per 60s of audio (fastest)

### HomeNoisy (39)

```bash
# Use filter value 39 for noisy home environments
curl -X POST "http://localhost:8080/convert?format=wav&filters=39" \
  --data-binary @input.mp3 \
  --output output.wav
```

**Best for:** Home recordings with TV/family in background

**Features:**

- Same as PhoneMobile
- Frequency filtering + noise reduction

**Processing:** ~700ms-1.5s per 60s of audio

### ClassroomVeryNoisy (127) - Use Sparingly

```bash
# Use filter value 127 for maximum noise reduction
curl -X POST "http://localhost:8080/convert?format=wav&filters=127" \
  --data-binary @input.mp3 \
  --output output.wav
```

**Best for:** Extremely noisy classrooms (30+ students)

**Warning:** Includes deesser which may affect Chinese tonal distinctions

**Processing:** ~2-3s per 60s of audio

## Filter Details

### Speech Mode (bit 6 = 64)

**Only works when Denoiser (bit 2 = 4) is enabled.**

Enables intelligent voice activity detection:

- **During speech**: Gentle noise reduction (~-20dB)
- **During silence**: Aggressive noise reduction (~-30dB)
- **Best for**: Fragmented speech with pauses, ums, ahs
- **Ideal for**: Language learning, interviews, classroom recordings

**Comparison:**

```
filters=4   → Denoiser with white noise mode (nt=w) - constant processing
filters=68  → Denoiser with speech mode (nt=s) - adaptive processing (4+64)
```

**When to use speech mode:**

- ✅ Student speech with pauses and hesitation
- ✅ Background students talking
- ✅ Classroom environment with intermittent noise
- ❌ Continuous speech without pauses
- ❌ Music or singing
- ❌ Very quiet/whispered content

### Filter Parameters

**Highpass** (`highpass=f=75:p=1`)

- `f=75`: Cutoff frequency 75Hz
- `p=1`: First-order filter (gentle 6dB/octave slope)

**Lowpass** (`lowpass=f=7500:p=1`)

- `f=7500`: Cutoff frequency 7.5kHz
- `p=1`: First-order filter (gentle rolloff)

**Denoiser** (`afftdn=nf=-25:nt=w` or `nt=s`)

- `nf=-25`: Noise floor -25dB
- `nt=w`: White noise mode (constant)
- `nt=s`: Speech mode (adaptive)

**Declick** (`adeclick=t=2:w=10`)

- `t=2`: Threshold sensitivity (conservative)
- `w=10`: Window size 10ms

**Deesser** (`deesser`)

- Uses default parameters
- Targets 6-8kHz sibilant range

**Normalize** (`dynaudnorm`)

- Frame-based dynamic normalization
- Prevents clipping, boosts quiet sections

## Examples

### Basic Conversion (cURL)

```bash
# Convert to WAV (default, no filters)
curl -X POST http://localhost:8080/convert?format=wav \
  --data-binary @input.mp3 \
  --output output.wav

# Convert to MP3 with basic cleanup
curl -X POST "http://localhost:8080/convert?format=mp3&filters=7" \
  --data-binary @input.mp3 \
  --output output.mp3
```

### With Custom Filters

```bash
# Highpass + Denoiser + Normalize (1 + 4 + 32 = 37)
curl -X POST "http://localhost:8080/convert?format=wav&filters=37" \
  --data-binary @input.mp3 \
  --output output.wav
```

### JavaScript/TypeScript Example

```typescript
async function convertAudio(file: File, filters: number = 0): Promise<Blob> {
  const format = 'wav';
  const query = new URLSearchParams({ format, filters: filters.toString() });
  
  const response = await fetch(`http://localhost:8080/convert?${query}`, {
    method: 'POST',
    body: file,
  });
  
  if (!response.ok) {
    throw new Error(`Conversion failed: ${response.statusText}`);
  }
  
  return response.blob();
}
```

## Supported Input Formats

- MP3, AAC, OGG (Vorbis, Opus)
- FLAC, WAV (PCM)
- MP4/M4A/MOV, WebM, Matroska (MKV)

## Performance

Processing time per 60s of audio:

| Filter Complexity         | Time Range |
| ------------------------- | ---------- |
| No filters (0)            | 100-200ms  |
| Single filter (1-32)      | 200-500ms  |
| Multiple filters (7-47)   | 500ms-2s   |
| With speech mode (68-127) | 1-3s       |

Times scale linearly with audio duration.

## Building & Running

### Docker (Recommended for Production)

```bash
# Build
docker build -t ffmpeg-audio-extractor .

# Run
docker run -d -p 8080:8080 ffmpeg-audio-extractor

# Run with verbose logging
docker run -d -p 8080:8080 -e VERBOSE=true ffmpeg-audio-extractor

# Run with increased upload size limit (e.g., for large video files)
docker run -d -p 8080:8080 -e MAX_UPLOAD_SIZE=1GB ffmpeg-audio-extractor
```

### Standalone Binary (Development)

```bash
# Build ffmpeg and the standalone binary for current platform
./build-standalone.sh --local

# Run (binary will be in dist/, includes platform suffix)
./dist/audio-extractor-$(go env GOOS)-$(go env GOARCH)

# Run with verbose logging
./dist/audio-extractor-$(go env GOOS)-$(go env GOARCH) -verbose
```

## Configuration

### Command Line Flags

| Flag | Description |
|------|-------------|
| `--version` | Print version information and exit |
| `--verbose`, `-v` | Enable verbose logging of ffmpeg output |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PORT` | HTTP server port | `8080` |
| `VERBOSE` | Enable verbose logging (`true` or `1`) | `false` |
| `MAX_UPLOAD_SIZE` | Maximum upload size (e.g., `250MB`, `1GB`, `524288000`) | `250MB` |

### Limits

- **Maximum upload size:** 250MB (configurable via `MAX_UPLOAD_SIZE`)
- **Processing timeout:** 5 minutes
- **Supported formats:** MP3, AAC, OGG, FLAC, WAV, MP4/M4A, WebM, MKV

## Security Considerations

⚠️ **This service is designed to run behind a reverse proxy with authentication.**

- No built-in authentication - secure at the proxy level (nginx, traefik, etc.)
- File size limits enforced (250MB default, configurable via MAX_UPLOAD_SIZE)
- Processing timeouts prevent resource exhaustion
- Temporary files are securely created and cleaned up
- FFmpeg runs with minimal privileges in Docker (scratch image)

**Recommended deployment:** Place behind a reverse proxy with rate limiting and authentication.

## Technical Details

**FFmpeg Version:** 8.0 (released August 2025)
**Base Image:** Alpine Linux 3.22
**Runtime:** Scratch (minimal)
**Container Size:** ~15MB

## Why These Settings?

**No Deesser by default:** Chinese has tonal distinctions that rely on subtle frequency shifts (6-8kHz range where deesser operates). Standard deesser can damage important acoustic cues for:

- Tone distinctions (high level vs falling)
- Retroflex consonants (zh, ch, sh)
- Aspirated vs unaspirated stops

**Speech Mode for Classroom:** Chinese language learning involves fragmented speech with:

- Thinking pauses while forming sentences
- Ums and ahs while searching for words
- Stop-start patterns during learning

Speech mode uses these natural pauses to learn the noise profile and applies different levels of reduction during speech vs silence, resulting in cleaner transcription.

## Source Code

- **[main.go](./main.go)** - Go HTTP server with audio conversion endpoints
- **[ffmpeg_docker.go](./ffmpeg_docker.go)** - Docker build initialization (uses system ffmpeg)
- **[ffmpeg_embedded.go](./ffmpeg_embedded.go)** - Embedded ffmpeg binary extraction for standalone builds
- **[scripts/](./scripts/)** - Build scripts for ffmpeg and Docker
