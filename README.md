# Audio Extractor Service

Audio conversion optimized for Dual Language Immersion classroom recordings with adaptive noise reduction for Chinese language learning.

## Quick Start

```typescript
import {
  DLIStudentPresets,
  selectPresetAuto,
  buildConversionQuery,
} from '@f/server/cloudflare/containers/audio-extractor';

// Auto-detect best preset (recommended)
const filters = selectPresetAuto();

// Or use specific preset
const filters = DLIStudentPresets.ClassroomChromebook; // Default for classroom

// Convert audio
const query = buildConversionQuery({format: 'wav', filters});
const response = await fetch(`${containerUrl}/convert?${query}`, {
  method: 'POST',
  body: audioFile,
});
```

## API

### POST /convert

**Query Parameters:**

- `format` - Output format: `wav` (default), `mp3`, or `flac`
- `filters` - Filter bitmask (see Filter Bitmask below)

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

### ClassroomChromebook (111) - **DEFAULT**

```typescript
const filters = DLIStudentPresets.ClassroomChromebook; // 111
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

```typescript
const filters = DLIStudentPresets.ClassroomSafe; // 47
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

```typescript
const filters = DLIStudentPresets.PhoneMobile; // 39
```

**Best for:** iPhone/Android recordings

**Features:**

- Removes hand-holding rumble and movement noise
- Handles phone codec compression artifacts
- Removes wind noise if outdoors

**Processing:** ~700ms-1.5s per 60s of audio

### HomeQuiet (36)

```typescript
const filters = DLIStudentPresets.HomeQuiet; // 36
```

**Best for:** Quiet home recordings (best quality scenario)

**Features:**

- Minimal processing preserves voice quality
- Gentle noise reduction
- Volume balancing

**Processing:** ~300-700ms per 60s of audio (fastest)

### HomeNoisy (39)

```typescript
const filters = DLIStudentPresets.HomeNoisy; // 39
```

**Best for:** Home recordings with TV/family in background

**Features:**

- Same as PhoneMobile
- Frequency filtering + noise reduction

**Processing:** ~700ms-1.5s per 60s of audio

### ClassroomVeryNoisy (127) - Use Sparingly

```typescript
const filters = DLIStudentPresets.ClassroomVeryNoisy; // 127
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

## Auto-Detection

Let the service automatically select the best preset:

```typescript
import {selectPresetAuto} from '@f/server/cloudflare/containers/audio-extractor';

// Auto-detect from browser user agent
const filters = selectPresetAuto();

// Explicitly specify environment
const filters = selectPresetAuto({environment: 'classroom'});

// Use microphone noise level analysis (0-1)
const filters = selectPresetAuto({noiseLevel: 0.7});
```

**Detection logic:**

1. If Chromebook user agent → ClassroomChromebook (111)
2. If phone user agent → PhoneMobile (39)
3. If desktop + high noise → HomeNoisy (39)
4. Otherwise → HomeQuiet (36)

## Helper Functions

```typescript
import {
  getPresetLabel,
  getFilterNames,
  getEstimatedProcessingTime,
  buildConversionQuery,
  validateFilterMask,
} from '@f/server/cloudflare/containers/audio-extractor';

// Get human-readable preset name
getPresetLabel(111); // "Classroom (Chromebook)"

// Get list of enabled filters
getFilterNames(111); // ['Highpass', 'Lowpass', 'Denoiser (speech mode)', 'Declick', 'Normalize']

// Estimate processing time
getEstimatedProcessingTime(111, 60); // { min: 1.5, max: 2.5 }

// Build URL query string
buildConversionQuery({format: 'wav', filters: 111}); // "format=wav&filters=111"

// Validate filter mask
const result = validateFilterMask(111);
if (!result.valid) {
  console.error(result.error);
}
```

### Preset Information

```typescript
import {
  getPresetLabel,
  getFilterNames,
  getEstimatedProcessingTime,
} from '@f/server/cloudflare/containers/audio-extractor';

getPresetLabel(111); // "Classroom (Chromebook)"
getFilterNames(111); // ['Highpass', 'Lowpass', 'Denoiser (speech mode)', 'Declick', 'Normalize']
getEstimatedProcessingTime(111, 60); // { min: 1.5, max: 2.5 }
```

### Validation

```typescript
import {validateFilterMask} from '@f/server/cloudflare/containers/audio-extractor';

const result = validateFilterMask(111);
if (!result.valid) {
  console.error(result.error);
}
```

## Examples

### Basic Conversion

```typescript
const response = await fetch(`${url}/convert?format=wav`, {
  method: 'POST',
  body: audioFile,
});
```

### With Custom Filters

```typescript
// Highpass + Denoiser + Normalize (1 + 4 + 32 = 37)
const filters = 37;
const response = await fetch(`${url}/convert?format=wav&filters=${filters}`, {
  method: 'POST',
  body: audioFile,
});
```

### React Hook

```typescript
import {useState} from 'react';
import {buildConversionQuery} from '@f/server/cloudflare/containers/audio-extractor';

function useAudioConverter(containerUrl: string) {
  const [isConverting, setIsConverting] = useState(false);

  const convert = async (file: File, filters: number = 0) => {
    setIsConverting(true);
    try {
      const query = buildConversionQuery({format: 'wav', filters});
      const response = await fetch(`${containerUrl}/convert?${query}`, {
        method: 'POST',
        body: file,
      });
      return await response.blob();
    } finally {
      setIsConverting(false);
    }
  };

  return {convert, isConverting};
}
```

## Supported Input Formats

- MP3, AAC, OGG (Vorbis, Opus)
- FLAC, WAV (PCM)
- MP4/M4A, WebM, Matroska (MKV)

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

```bash
# Build
docker build -t ffmpeg-audio-extractor .

# Run
docker run -d -p 8080:8080 ffmpeg-audio-extractor
```

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

- **[filters.ts](./filters.ts)** - TypeScript constants and helper functions
- **[index.ts](./index.ts)** - Container class with inline documentation
- **[main.go](./main.go)** - Go server implementation
