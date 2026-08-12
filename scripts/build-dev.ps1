<#
.SYNOPSIS
    Dev build: compiles cmd/kairos and copies the sherpa-onnx-go-windows
    runtime DLLs next to the binary, so it's actually runnable without going
    through the full MSI packaging pipeline (packaging/build.ps1).

.DESCRIPTION
    go build alone only links these libs into the import table — it does not
    copy the DLLs sherpa-onnx-go-windows dynamically loads at runtime
    (onnxruntime.dll / sherpa-onnx-c-api.dll / sherpa-onnx-cxx-api.dll),
    which is why the built exe fails at startup with
    "sherpa-onnx-c-api.dll not found" unless they're copied manually.

    CPU-only by design (user-directed decision, 2026-08-11): a CUDA
    execution provider build was tried and reverted — CUDA Toolkit/cuDNN
    version mismatches don't fail cleanly, they crash the whole process
    with an unhandled native exception (see
    internal/asr/paraformer_windows.go's NewParaformerTranscriber doc
    comment). CPU has no such external runtime dependency.

.NOTES
    Does NOT copy FFmpeg or ASR model files — see
    docs/scratch/short-drama-highlight-clip/windows-bringup-runbook.md and
    scripts/download-models.ps1.
#>

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$outDir = $root
New-Item -ItemType Directory -Path $outDir -Force | Out-Null

Write-Host "==> go build"
go build -o (Join-Path $outDir "kairos.exe") (Join-Path $root "cmd\kairos")

Write-Host "==> Copying sherpa-onnx-go-windows runtime DLLs (CPU-only)"
$modDir = Join-Path (go env GOMODCACHE) "github.com\k2-fsa\sherpa-onnx-go-windows@v1.13.4\lib\x86_64-pc-windows-gnu"
if (-not (Test-Path $modDir)) {
    throw "sherpa-onnx-go-windows module cache not found at $modDir. Run 'go mod download' first."
}
Copy-Item (Join-Path $modDir "*.dll") $outDir -Force

Write-Host "==> Done: $outDir\kairos.exe"
