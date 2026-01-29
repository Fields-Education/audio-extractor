# Changelog

## [1.4.0](https://github.com/Fields-Education/audio-extractor/compare/v1.3.0...v1.4.0) (2026-01-29)


### Features

* add --local flag to build-standalone.sh for faster local builds ([fe9afc7](https://github.com/Fields-Education/audio-extractor/commit/fe9afc797ab001faba035a829e79904504d664a2))
* publish release only after all assets and docker images are ready ([6d33adc](https://github.com/Fields-Education/audio-extractor/commit/6d33adc432191b96abc0bd1548dfd12540ac98d1))


### Bug Fixes

* add lame include/lib paths for macOS ffmpeg build ([3e5e537](https://github.com/Fields-Education/audio-extractor/commit/3e5e5376bdd855858510661c41c8c1324f592a46))
* group all non-major updates into single PR ([fade4fe](https://github.com/Fields-Education/audio-extractor/commit/fade4fe5c6e0d90dd05905183bfda422958b9f73))

## [1.3.0](https://github.com/Fields-Education/audio-extractor/compare/v1.2.0...v1.3.0) (2026-01-29)


### Features

* add mp3 muxer support and centralize ffmpeg build config ([a394d2f](https://github.com/Fields-Education/audio-extractor/commit/a394d2fd2f073263748874955c6eebdfbe3276fa))

## [1.2.0](https://github.com/Fields-Education/audio-extractor/compare/v1.1.1...v1.2.0) (2026-01-29)


### Features

* add ccache for faster ffmpeg compilation ([bc5b13c](https://github.com/Fields-Education/audio-extractor/commit/bc5b13ce07a377ea0f77efd9daa818a4435003eb))

## [1.1.1](https://github.com/Fields-Education/audio-extractor/compare/v1.1.0...v1.1.1) (2026-01-29)


### Bug Fixes

* disable provenance/sbom for Docker builds to fix manifest merge ([7a6949d](https://github.com/Fields-Education/audio-extractor/commit/7a6949d9aa60172326fc70fb0f80238fdeca893c))

## [1.1.0](https://github.com/Fields-Education/audio-extractor/compare/v1.0.1...v1.1.0) (2026-01-29)


### Features

* add automated Docker multi-arch builds with ECR push ([0a818b3](https://github.com/Fields-Education/audio-extractor/commit/0a818b3c93a69b78b2d37c7cd841f35a6025c4c4))
* support PORT environment variable ([1bbf15d](https://github.com/Fields-Education/audio-extractor/commit/1bbf15dd112ed4a3ee3c5597f511480143303427))


### Bug Fixes

* use Blacksmith native runners for faster Linux builds ([0e5f20a](https://github.com/Fields-Education/audio-extractor/commit/0e5f20ac4ccba55e09b4e9f9f684f5adf42be32c))

## [1.0.1](https://github.com/Fields-Education/audio-extractor/compare/v1.0.0...v1.0.1) (2026-01-29)


### Bug Fixes

* use macos runner for darwin builds ([bb5c999](https://github.com/Fields-Education/audio-extractor/commit/bb5c9995981dbf89f66ad94abc8e19b45d7adfcb))

## 1.0.0 (2026-01-29)


### Features

* add standalone binary support with embedded ffmpeg ([f2b1792](https://github.com/Fields-Education/audio-extractor/commit/f2b17928473f86604197eec46a0dd6be7b4e9a7a))
