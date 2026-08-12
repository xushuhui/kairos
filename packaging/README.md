# Ticket 09 — Packaging & Distribution

Status: **scaffolding only, unverified.** `go-msi` requires a Windows machine
with WiX Toolset >= 3.10 on `PATH` (its own README's stated requirement) —
neither exists on this repo's macOS dev machine, so nothing under this
directory has actually been run. What's here is a complete, config-driven
starting point, not a tested pipeline.

## What's in this directory

- `wix.json` — [go-msi](https://github.com/mh-cbon/go-msi) manifest: declares
  the files/directories to bundle, a `PATH` env-var entry (so `internal/video`'s
  `exec.Command("ffmpeg", ...)` / `exec.LookPath` calls resolve without the
  user installing FFmpeg separately), and a Start Menu shortcut.
- `build.ps1` — PowerShell script that assembles a `dist/` staging directory
  (binary + FFmpeg + model files + sherpa-onnx runtime DLLs) and invokes
  `go-msi make`.

## Checklist status (issues/09-packaging-distribution.md)

- [ ] **MSI installs on a clean Windows 10/11 box** — not run, no Windows host available.
- [ ] **Works immediately, no extra downloads** — not run.
- [ ] **Verified on both Windows 10 (1903+) and Windows 11** — not run.
- [x] **No CUDA/cuDNN DLLs to bundle** — `internal/asr` is CPU-only by design (user-directed decision, 2026-08-11: a CUDA execution provider build was tried and reverted — version-mismatched CUDA/cuDNN doesn't fail cleanly, it crashes the whole process with an unhandled native exception; see `internal/asr/paraformer_windows.go`'s `NewParaformerTranscriber` doc comment). `wix.json` bundles the 3 sherpa-onnx CPU runtime DLLs (`onnxruntime.dll`/`sherpa-onnx-c-api.dll`/`sherpa-onnx-cxx-api.dll`, sourced from the `sherpa-onnx-go-windows` Go module cache) instead — no external runtime dependency, no version pin to get wrong.
- [x] **No code-signing certificate purchased** — already the confirmed decision (05-deployment-cicd.md); nothing to build here, `build.ps1` doesn't sign anything.
- [x] **First-launch CUDA detection, degrade to CPU + notify user** — moot for ASR (CPU-only now, no detection needed); FFmpeg's own NVENC/NVDEC path (independent of ONNX Runtime, goes through the NVIDIA driver directly) still self-detects: `cmd/kairos/app.go`'s `cudaStatusText()` calls `video.CudaAvailable()` on startup and shows a status line, `internal/video`'s `SelectEncoder`/`CutClip` fall back to `libx264` on their own (ticket 02).

## What you need to supply before running `build.ps1`

None of these exist in this repo — they're real binary artifacts this
sandbox has no way to obtain or verify:

- **FFmpeg**: official Windows `ffmpeg.exe`/`ffprobe.exe` build (per
  05-deployment-cicd.md: "安装包内置官方 Windows pre-built 二进制").
- **Model files**: Paraformer-large + Silero VAD + punctuation restoration
  ONNX model files (see `issues/03-kairos-asr-paraformer.md` and spec.md's
  "本地 ASR 实现" section for licensing — FunASR Model License, requires
  Alibaba/FunAudioLLM attribution, already noted in the top-level README).
- **`LICENSE.rtf`**: this project's own license, converted to RTF/Windows-1252
  via `go-msi to-rtf` — the top-level README already flags that no license
  has been chosen for this repo yet, so this file can't be produced either.
- **WiX Toolset >= 3.10** and **`go-msi`** itself, installed on the Windows
  build machine and on `PATH`.

The sherpa-onnx runtime DLLs (`onnxruntime.dll`/`sherpa-onnx-c-api.dll`/
`sherpa-onnx-cxx-api.dll`) do NOT need supplying separately — `build.ps1`
copies them straight from the `sherpa-onnx-go-windows` Go module cache
(same source `scripts/build-dev.ps1` uses for local dev builds), so
`go mod download` having run once is the only prerequisite for that part.

## Running it (once the above exist, on Windows)

```powershell
pwsh ./packaging/build.ps1 `
  -FfmpegDir  C:\path\to\ffmpeg-bin `
  -ModelsDir  C:\path\to\asr-models `
  -Version    0.1.0
```

First run only: `go-msi set-guid --path wix.json` (already called by
`build.ps1`, but only actually assigns GUIDs the first time — after that they
persist in `wix.json` and must be committed, not regenerated per release, or
Windows Installer will treat every release as an unrelated product for
upgrade/uninstall purposes).
