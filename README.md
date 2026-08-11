# Kairos

A Windows desktop tool that turns a short-drama episode (1–3 min MP4/MOV/AVI/WEBM) into an ad-hook highlight clip, fully automatically: local ASR transcribes the dialogue, a cloud LLM picks the most compelling continuous window, and FFmpeg cuts it out — no manual editing, no cloud video upload.

中文说明见 [README.zh.md](./README.zh.md).

## Pipeline

```
source video ──▶ FFmpeg: extract 16kHz mono WAV
                        │
                        ▼
             local ASR (FunASR Paraformer-large, ONNX + CUDA)
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

Target platform is Windows 10 (1903+) / Windows 11 with an NVIDIA GPU. This repo is developed and tested on macOS, so anything gated behind CUDA/Windows-only bindings is implemented but **unverified** until run on the real target hardware — that's called out explicitly below, not glossed over.

| # | Package | What it does | Status |
|---|---|---|---|
| 01 | `internal/core` | Domain types, `Transcriber`/`HighlightJudge` interfaces, `ComputeWindow()` | ✅ Done, tested |
| 02 | `internal/video` | FFmpeg subprocess wrapper: audio extraction, CUDA detection, clip cutting, duration probe | ✅ Done, tested |
| 03 | `internal/asr` | Local Paraformer-large + Silero VAD transcription via `sherpa-onnx-go-windows` | ⚠️ Word→sentence merge logic is cross-platform and tested; the real sherpa-onnx integration is gated behind `//go:build windows` and **cannot be verified on this machine** (no Windows host, no CUDA, no model files) |
| 04 | `internal/llm` | DeepSeek V4-flash client implementing `HighlightJudge` | ✅ Done, tested against a mock server; a live API call needs a real `DEEPSEEK_API_KEY`, not yet exercised |
| 05 | `internal/core` | `RunHighlightExtraction()` — the orchestration entry point | ✅ Done, tested with hand-written fakes for the two injected interfaces |
| 06 | — | Real-material end-to-end validation | ⬜ Not started — blocked on a Windows host, NVIDIA GPU, real ASR model files, and a DeepSeek API key |
| 07 | `internal/history` | JSON sidecar history records | ✅ Done, tested |
| 08 | `cmd/kairos` | Fyne GUI | ⬜ Not started — blocked on 06 |
| 09 | — | MSI packaging | ⬜ Not started — blocked on 08 |

Full ticket definitions and design decisions live under [`docs/scratch/short-drama-highlight-clip/`](./docs/scratch/short-drama-highlight-clip/) — `spec.md` for product decisions, `implementation-plan.md` for the engineering design, `map.md` for the decision log.

## Requirements

- Go 1.26+
- On Windows: a cgo-capable C compiler (MinGW-w64 gcc), required to build `fyne.io/fyne/v2` (`go-gl/gl`) and `github.com/k2-fsa/sherpa-onnx-go-windows`. One-time setup: `Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned` (allows local scripts to run), then `.\scripts\setup-cgo-toolchain.ps1`.
- `ffmpeg`/`ffprobe` on `PATH` (dev/test only — the shipped installer bundles them)
- To build/run for real on Windows: an NVIDIA GPU with CUDA support (falls back to CPU/`libx264` if unavailable) and the Paraformer-large + Silero VAD model files
- A DeepSeek API key for `internal/llm`

## Build & test

```sh
go build ./...
go test ./...
```

Most tests spin up real FFmpeg subprocesses against a small generated fixture (silent, static-color 5s MP4) rather than mocking FFmpeg — see `spec.md`'s Testing Decisions for why. Tests are skipped gracefully if `ffmpeg`/`ffprobe` aren't on `PATH`.

## Project layout

```
kairos/
├── cmd/kairos/          binary entry point (Fyne GUI, not yet implemented)
├── internal/
│   ├── core/            orchestration, domain types, ComputeWindow(), the two test seams
│   ├── video/            FFmpeg subprocess wrapper
│   ├── asr/              Paraformer transcription (Windows-only for the real backend)
│   ├── llm/               DeepSeek client
│   ├── history/           JSON sidecar history records
│   └── testutil/          shared FFmpeg test-fixture helpers
└── docs/scratch/short-drama-highlight-clip/   full spec and design docs
```

## Configuration

API key and output settings live in a local JSON file (`os.UserConfigDir()/kairos/config.json`, i.e. `%APPDATA%\kairos\config.json` on Windows):

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
- `github.com/sashabaranov/go-openai`, `github.com/k2-fsa/sherpa-onnx-go-windows` — see their respective repositories for license terms.

This repository's own license has not been chosen yet.
