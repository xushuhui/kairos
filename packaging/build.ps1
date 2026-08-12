# packaging/build.ps1 — assembles the dist/ staging directory and invokes
# go-msi. MUST run on Windows (go-msi itself requires WiX >= 3.10 on PATH,
# neither of which exist on this repo's macOS dev machine — this script is
# written but genuinely unverified, see packaging/README.md).
#
# CPU-only by design (user-directed decision, 2026-08-11): a CUDA execution
# provider build was tried and reverted for internal/asr — see
# internal/asr/paraformer_windows.go's NewParaformerTranscriber doc comment
# for why. No CUDA/cuDNN runtime DLLs to bundle here as a result (FFmpeg's
# own NVENC/NVDEC hardware encode/decode path is unaffected either way —
# that goes through the NVIDIA driver directly, not ONNX Runtime's
# execution provider, and isn't part of this change).
#
# Usage: pwsh ./packaging/build.ps1 -Version 0.1.0
#   -FfmpegDir   directory containing ffmpeg.exe/ffprobe.exe            (required)
#   -ModelsDir   directory containing the Paraformer/Silero-VAD/        (required)
#                punctuation model files, copied to dist/models/
#   -Version     product version passed to go-msi                      (required)

param(
    [Parameter(Mandatory = $true)][string]$FfmpegDir,
    [Parameter(Mandatory = $true)][string]$ModelsDir,
    [Parameter(Mandatory = $true)][string]$Version
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dist = Join-Path $root "packaging/dist"

Write-Host "==> Cleaning dist/"
Remove-Item -Recurse -Force $dist -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $dist | Out-Null
New-Item -ItemType Directory -Path (Join-Path $dist "models") | Out-Null

Write-Host "==> Building kairos.exe"
Push-Location $root
go build -o (Join-Path $dist "kairos.exe") ./cmd/kairos
Pop-Location

Write-Host "==> Copying FFmpeg"
Copy-Item (Join-Path $FfmpegDir "ffmpeg.exe") $dist
Copy-Item (Join-Path $FfmpegDir "ffprobe.exe") $dist

Write-Host "==> Copying ASR model files"
Copy-Item (Join-Path $ModelsDir "*") (Join-Path $dist "models") -Recurse

Write-Host "==> Copying sherpa-onnx-go-windows runtime DLLs (CPU-only)"
# go build alone only links these into the import table — it does not copy
# the DLLs sherpa-onnx-go-windows dynamically loads at runtime, same reason
# scripts/build-dev.ps1 copies them for local dev builds.
$modDir = Join-Path (go env GOMODCACHE) "github.com\k2-fsa\sherpa-onnx-go-windows@v1.13.4\lib\x86_64-pc-windows-gnu"
if (-not (Test-Path $modDir)) {
    throw "sherpa-onnx-go-windows module cache not found at $modDir. Run 'go mod download' first."
}
Copy-Item (Join-Path $modDir "*.dll") $dist -Force

Write-Host "==> Running go-msi"
Push-Location (Join-Path $root "packaging")
go-msi set-guid --path wix.json  # no-op after the first run; guids persist in wix.json once generated
go-msi make --path wix.json --msi "kairos-$Version.msi" --version $Version --license LICENSE.rtf
Pop-Location

Write-Host "==> Done: packaging/kairos-$Version.msi"
