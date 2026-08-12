# Kairos

A Windows desktop tool that turns a short-drama episode (1–3 min MP4/MOV/AVI/WEBM) into an ad-hook highlight clip, fully automatically: local ASR transcribes the dialogue, a cloud LLM picks the most compelling continuous window, and FFmpeg cuts it out — no manual editing, no cloud video upload.

中文说明见 [README.zh.md](./README.zh.md).

## Pipeline

```
source video ──▶ FFmpeg: extract 16kHz mono WAV
                        │
                        ▼
             local ASR (FunASR Paraformer-large, ONNX, CPU)
                        │  sentence list with per-sentence ms timestamps
                        ▼
             cloud LLM (DeepSeek V4-flash) — judges the best ad-hook window,
             returns start/end sentence IDs + reasoning
                        │
                        ▼
             code maps sentence IDs → ms timestamps, anchors the climax
             at the clip's end, clamps to video boundaries
                        │
                        ▼
             FFmpeg: hardware-accelerated cut (NVENC, falls back to libx264)
                        │
                        ▼
             ~60s highlight clip on local disk
```

Everything runs on one machine, single user, no server infrastructure. The only network call is the LLM judge request, which sends plain dialogue text — never the video itself.

## Status

Target platform for actual deployment is Windows 10 (1903+) / Windows 11 — that decision hasn't changed (see `map.md`'s "Out of scope"). `internal/asr` also ships a mirrored macOS (darwin) backend purely as a **local dev/test convenience**, user-directed: verify the real pipeline end-to-end on this machine before installing on the Windows target, instead of the slow "make a Go change → someone rebuilds on Windows → pastes back a log" loop. The Windows and macOS backends are the same Go logic against two platform-specific sherpa-onnx bindings with an API surface diffed and confirmed identical (`sherpa-onnx-go-windows@v1.13.4` vs `sherpa-onnx-go-macos@v1.13.5`) — verifying on macOS gives high confidence for Windows, but isn't a substitute for a real Windows run.

| # | Package | What it does | Status |
|---|---|---|---|
| 01 | `internal/core` | Domain types, `Transcriber`/`HighlightJudge` interfaces, `ComputeWindow()` | ✅ Done, tested |
| 02 | `internal/video` | FFmpeg subprocess wrapper: audio extraction, CUDA detection, clip cutting, duration probe | ✅ Done, tested |
| 03 | `internal/asr` | Local Paraformer-large + Silero VAD transcription via `sherpa-onnx-go-windows`/`-macos` (CPU provider only) | ✅ Verified end-to-end on macOS (real models via `scripts/download-models.sh`, real TTS-generated Chinese speech audio) — VAD segmentation, Paraformer decode, and punctuation restoration all produce correct output. Windows build is the identical Go logic against the platform-matched binding; not independently run on real Windows hardware |
| 04 | `internal/llm` | DeepSeek V4-flash client implementing `HighlightJudge` | ✅ Done, tested against a mock server; a live API call needs a real `DEEPSEEK_API_KEY`, not yet exercised |
| 05 | `internal/core` | `RunHighlightExtraction()` — the orchestration entry point | ✅ Done, tested with hand-written fakes for the two injected interfaces |
| 06 | — | Real-material end-to-end validation | ⚠️ ASR itself verified (see 03); full GUI → DeepSeek → clip-cut chain on a real short-drama file still needs a real Windows run with a real API key |
| 07 | `internal/history` | JSON sidecar history records | ✅ Done, tested |
| 08 | `cmd/kairos` | Fyne GUI | ✅ Implemented, builds on both Windows and macOS; tested headless via Fyne's test driver with fake `Transcriber`/`HighlightJudge`. Not yet launched as a live window on real hardware |
| 09 | — | MSI packaging | ⬜ Not started — scaffolding only, needs a real Windows + WiX Toolset host |

Full ticket definitions and design decisions live under [`docs/scratch/short-drama-highlight-clip/`](./docs/scratch/short-drama-highlight-clip/) — `spec.md` for product decisions, `implementation-plan.md` for the engineering design, `map.md` for the decision log.

## Requirements

- Go 1.26+
- On Windows: a cgo-capable C compiler (MinGW-w64 gcc), required to build `fyne.io/fyne/v2` (`go-gl/gl`) and `github.com/k2-fsa/sherpa-onnx-go-windows`. One-time setup: `Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned` (allows local scripts to run), then `.\scripts\setup-cgo-toolchain.ps1`.
- On macOS (dev/test only, see Status above): Xcode Command Line Tools for cgo (`xcode-select --install`), usually already present. No extra toolchain step beyond that — `sherpa-onnx-go-macos`'s cgo linking uses an rpath baked in at build time, so unlike Windows there's no DLL-copy step needed either.
- `ffmpeg`/`ffprobe` on `PATH` (dev/test only — the shipped installer bundles them for the real Windows deployment)
- To build/run for real on Windows: an NVIDIA GPU with CUDA support for FFmpeg's `-hwaccel cuda`/`h264_nvenc` (falls back to CPU/`libx264` if unavailable — `internal/asr` itself is CPU-only on both platforms, see Status), and the Paraformer-large + Silero VAD model files
- A DeepSeek API key for `internal/llm`

## Build & test

```sh
go build ./...
go test ./...
```

Most tests spin up real FFmpeg subprocesses against a small generated fixture (silent, static-color 5s MP4) rather than mocking FFmpeg — see `spec.md`'s Testing Decisions for why. Tests are skipped gracefully if `ffmpeg`/`ffprobe` aren't on `PATH`.

### Real ASR pipeline testing (macOS or Windows)

`internal/asr`'s actual sherpa-onnx integration needs real model files (~500 MB, not vendored — see `packaging/README.md`) and real speech audio, so it's gated behind two env vars and skipped in the normal suite above:

```sh
./scripts/download-models.sh          # fetches models/ once, idempotent (Windows: scripts/download-models.ps1)
say -v Tingting -o /tmp/speech.aiff "随便一句中文测试语音"   # macOS only, any real speech WAV works
ffmpeg -y -i /tmp/speech.aiff -vn -ac 1 -ar 16000 -f wav /tmp/speech.wav

KAIROS_TEST_MODEL_DIR="$(pwd)/models" KAIROS_TEST_AUDIO_PATH=/tmp/speech.wav \
  go test ./internal/asr/... -run TestParaformerTranscriber_RealModels_ManualRun -v
```

## Project layout

```
kairos/
├── cmd/kairos/          binary entry point (Fyne GUI)
├── internal/
│   ├── core/            orchestration, domain types, ComputeWindow(), the two test seams
│   ├── video/            FFmpeg subprocess wrapper
│   ├── asr/              Paraformer transcription — real backend on Windows + macOS (dev/test), mirrored Go logic per platform binding
│   ├── llm/               DeepSeek client
│   ├── history/           JSON sidecar history records
│   ├── apppath/           resolves the exe-relative directory config/history/logs/models all live under
│   └── testutil/          shared FFmpeg test-fixture helpers
└── docs/scratch/short-drama-highlight-clip/   full spec and design docs
```

## Configuration

Kairos is self-contained/portable: config, history, and logs all live next to the running executable, not in a per-OS user profile directory. Given `kairos.exe` at some directory, the layout is:

```
<exe dir>/
├── kairos.exe
├── config.json      DeepSeek API key + output settings
├── history/         one JSON file per processed video (success or failure)
├── logs/
│   ├── app.log       full log (every level)
│   └── error.log     Error-level only, for quick manual inspection
└── models/           bundled Paraformer/Silero-VAD/punctuation ONNX models
```

`config.json`:

```json
{
  "deepseek": {
    "api_key": ""
  },
  "output": {
    "default_dir": ""
  }
}
```

## Third-party licenses and attribution

- ASR: FunASR Paraformer-large model weights — **FunASR Model License**, commercial use requires attribution to Alibaba / FunAudioLLM. This notice satisfies that requirement pending a proper in-app "About" screen (ticket 08).
- `github.com/sashabaranov/go-openai`, `github.com/k2-fsa/sherpa-onnx-go-windows`, `github.com/k2-fsa/sherpa-onnx-go-macos` (dev/test only) — see their respective repositories for license terms.

This repository's own license has not been chosen yet.
