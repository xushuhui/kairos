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
  (binary + FFmpeg + model files + CUDA DLLs) and invokes `go-msi make`.

## Checklist status (issues/09-packaging-distribution.md)

- [ ] **MSI installs on a clean Windows 10/11 box** — not run, no Windows host available.
- [ ] **Works immediately, no extra downloads** — depends on the CUDA EP question below.
- [ ] **Verified on both Windows 10 (1903+) and Windows 11** — not run.
- [ ] **CUDA/cuDNN DLLs bundled, version-locked to the ONNX Runtime build** — see "CUDA execution provider: unresolved risk" below; the DLL filenames in `wix.json` are a best-effort guess, not a verified pin.
- [x] **No code-signing certificate purchased** — already the confirmed decision (05-deployment-cicd.md); nothing to build here, `build.ps1` doesn't sign anything.
- [x] **First-launch CUDA detection, degrade to CPU + notify user** — already implemented at the application level, not a packaging concern: `cmd/kairos/app.go`'s `cudaStatusText()` calls `video.CudaAvailable()` on startup and shows a status line; `internal/asr`'s `NewParaformerTranscriber` and `internal/video`'s `SelectEncoder`/`CutClip` already fall back to CPU/`libx264` on their own (tickets 02/03).

## CUDA execution provider: unresolved risk, found during this pass

`wix.json` bundles `cudart64_110.dll` / `cublas64_11.dll` / `cublasLt64_11.dll`
/ `cufft64_10.dll` / `curand64_10.dll` / `cudnn64_8.dll` — CUDA 11.8 /
cuDNN 8.x naming, based on sherpa-onnx's own build docs ("You need to install
CUDA toolkit 11.8" — <https://k2-fsa.github.io/sherpa/onnx/install/windows/build-cuda.html>).
**This is not independently verified against the exact `sherpa-onnx-go-windows
v1.13.4` release this repo depends on** (go.mod), for a concrete reason found
while writing this:

Inspecting the actual fetched module
(`$GOPATH/pkg/mod/github.com/k2-fsa/sherpa-onnx-go-windows@v1.13.4/lib/x86_64-pc-windows-gnu/`)
shows exactly three bundled DLLs: `onnxruntime.dll`, `sherpa-onnx-c-api.dll`,
`sherpa-onnx-cxx-api.dll` — **no separate `onnxruntime_providers_cuda.dll`**,
which is how most ONNX Runtime distributions ship CUDA execution-provider
support as an add-on to the base (CPU) runtime. Two possibilities:

1. This build's `onnxruntime.dll` has CUDA EP compiled in directly (some ORT
   build configurations do this), in which case the `cudart*`/`cublas*`/
   `cudnn*` DLLs above are exactly what it needs at load time and the
   version pin just needs confirming against whatever ORT version this
   specific `onnxruntime.dll` was built against.
2. This build is CPU-only, and `internal/asr.NewParaformerTranscriber(...,
   useCuda: true)` will fail to initialize the "cuda" provider and fall back
   to CPU (the fallback path ticket 03 already implements) — meaning the
   whole CUDA/cuDNN DLL bundle in this packaging step would be dead weight
   for the ASR half of the pipeline (FFmpeg's own `-hwaccel cuda`/`h264_nvenc`
   NVENC/NVDEC path is unaffected either way — that goes through FFmpeg's own
   CUDA bindings, completely independent of ONNX Runtime's execution
   provider).

**This can only be resolved by actually running `NewParaformerTranscriber`
with `useCuda: true` on a real Windows box with an NVIDIA GPU** — exactly
ticket 06's blocked prerequisite. Do that check before trusting this DLL list
for a real release; don't ship 09 on faith that the guessed versions are
right.

## What you need to supply before running `build.ps1`

None of these exist in this repo — they're real binary artifacts this
sandbox has no way to obtain or verify:

- **FFmpeg**: official Windows `ffmpeg.exe`/`ffprobe.exe` build (per
  05-deployment-cicd.md: "安装包内置官方 Windows pre-built 二进制").
- **Model files**: Paraformer-large + Silero VAD + punctuation restoration
  ONNX model files (see `issues/03-kairos-asr-paraformer.md` and spec.md's
  "本地 ASR 实现" section for licensing — FunASR Model License, requires
  Alibaba/FunAudioLLM attribution, already noted in the top-level README).
- **CUDA/cuDNN runtime DLLs**: see the unresolved-risk section above before
  trusting the filenames already in `wix.json`.
- **`NVIDIA_REDIST_LICENSE.txt`**: NVIDIA's actual redistribution license
  text for the bundled runtime DLLs (05-deployment-cicd.md requires this
  notice ship in the installer; not fabricated here).
- **`LICENSE.rtf`**: this project's own license, converted to RTF/Windows-1252
  via `go-msi to-rtf` — the top-level README already flags that no license
  has been chosen for this repo yet, so this file can't be produced either.
- **WiX Toolset >= 3.10** and **`go-msi`** itself, installed on the Windows
  build machine and on `PATH`.

## Running it (once the above exist, on Windows)

```powershell
pwsh ./packaging/build.ps1 `
  -FfmpegDir  C:\path\to\ffmpeg-bin `
  -ModelsDir  C:\path\to\asr-models `
  -CudaDir    C:\path\to\cuda-dlls `
  -Version    0.1.0
```

First run only: `go-msi set-guid --path wix.json` (already called by
`build.ps1`, but only actually assigns GUIDs the first time — after that they
persist in `wix.json` and must be committed, not regenerated per release, or
Windows Installer will treat every release as an unrelated product for
upgrade/uninstall purposes).
