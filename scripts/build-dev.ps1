<#
.SYNOPSIS
    Dev build: compiles cmd/kairos and copies the sherpa-onnx runtime DLLs
    next to the binary, so it's actually runnable without going through the
    full MSI packaging pipeline (packaging/build.ps1).

.DESCRIPTION
    go build alone only links these libs into the import table — it does not
    copy the DLLs sherpa-onnx-go-windows dynamically loads at runtime, which
    is why the built exe fails at startup with "sherpa-onnx-c-api.dll not
    found" unless they're copied manually.

    By default this downloads the official k2-fsa/sherpa-onnx v1.13.4
    Windows build with a real CUDA execution provider
    (sherpa-onnx-v1.13.4-cuda-12.x-cudnn-9.x-win-x64-cuda.tar.bz2 — same
    version tag as the sherpa-onnx-go-windows Go module pinned in go.mod,
    so the C API surface it's built against matches) and uses its 6 DLLs —
    onnxruntime.dll, onnxruntime_providers_{cuda,shared,tensorrt}.dll,
    sherpa-onnx-c-api.dll, sherpa-onnx-cxx-api.dll — instead of the
    CPU-only 3-DLL set bundled in the Go module cache. That CPU-only build
    has no CUDA execution provider compiled in at all — confirmed at
    runtime via sherpa-onnx's own stderr message: "Please compile with
    -DSHERPA_ONNX_ENABLE_GPU=ON...Fallback to cpu!".

.PARAMETER Cpu
    Skip the ~300MB CUDA download; use the CPU-only DLLs already sitting in
    the Go module cache instead (no GPU acceleration, nothing to fetch).

.NOTES
    Does NOT install CUDA Toolkit 12.x / cuDNN 9.x themselves —
    onnxruntime_providers_cuda.dll dynamically loads their runtime DLLs
    (cudart64_12.dll / cublas64_12.dll / cudnn64_9.dll etc.) at CUDA-EP init
    time. Without them on this machine, CUDA-EP init fails and
    internal/asr's existing fallback silently drops to CPU (same behavior
    as before, not a regression) — this script has no way to verify
    CUDA Toolkit/cuDNN are actually installed. Check logs\app.log after a
    run for "asr: paraformer recognizer initialized" provider=cuda|cpu to
    see which one actually loaded.

    Does NOT copy FFmpeg or ASR model files — see
    docs/scratch/short-drama-highlight-clip/windows-bringup-runbook.md and
    scripts/download-models.ps1.
#>

param(
    [switch]$Cpu
)

$ErrorActionPreference = "Stop"

$root = Split-Path -Parent $PSScriptRoot
$outDir = $root
New-Item -ItemType Directory -Path $outDir -Force | Out-Null

Write-Host "==> go build"
go build -o (Join-Path $outDir "kairos.exe") (Join-Path $root "cmd\kairos")

if ($Cpu) {
    Write-Host "==> Copying sherpa-onnx-go-windows CPU-only runtime DLLs"
    $modDir = Join-Path (go env GOMODCACHE) "github.com\k2-fsa\sherpa-onnx-go-windows@v1.13.4\lib\x86_64-pc-windows-gnu"
    if (-not (Test-Path $modDir)) {
        throw "sherpa-onnx-go-windows module cache not found at $modDir. Run 'go mod download' first."
    }
    Copy-Item (Join-Path $modDir "*.dll") $outDir -Force
} else {
    Write-Host "==> Copying sherpa-onnx CUDA-enabled runtime DLLs (CUDA 12.x + cuDNN 9.x)"
    # Version pinned to match sherpa-onnx-go-windows in go.mod — bump both
    # together, never independently (the Go binding's cgo declarations must
    # match the C API surface these DLLs actually export).
    $sherpaVersion = "1.13.4"
    $cudaArchiveName = "sherpa-onnx-v$sherpaVersion-cuda-12.x-cudnn-9.x-win-x64-cuda.tar.bz2"
    $cudaCacheDir = Join-Path $env:TEMP "kairos-sherpa-cuda-v$sherpaVersion"
    $cudaLibDir = Join-Path $cudaCacheDir "sherpa-onnx-v$sherpaVersion-cuda-12.x-cudnn-9.x-win-x64-cuda\lib"
    if (-not (Test-Path (Join-Path $cudaLibDir "onnxruntime.dll"))) {
        New-Item -ItemType Directory -Path $cudaCacheDir -Force | Out-Null
        $archive = Join-Path $env:TEMP $cudaArchiveName
        if (-not (Test-Path $archive)) {
            Write-Host "[download] $cudaArchiveName (~300MB, one-time — cached at $archive afterwards)..."
            Invoke-WebRequest -Uri "https://github.com/k2-fsa/sherpa-onnx/releases/download/v$sherpaVersion/$cudaArchiveName" -OutFile $archive
        }
        tar xf $archive -C $cudaCacheDir
    } else {
        Write-Host "[skip] CUDA DLLs already cached at $cudaLibDir"
    }
    Copy-Item (Join-Path $cudaLibDir "*.dll") $outDir -Force
}

Write-Host "==> Done: $outDir\kairos.exe"
if (-not $Cpu) {
    Write-Host "CUDA EP needs CUDA Toolkit 12.x + cuDNN 9.x installed on this machine (not installed by this script)."
    Write-Host "Falls back to CPU automatically if unavailable. Check logs\app.log for 'provider=cuda' vs 'provider=cpu' to confirm which one actually loaded."
}
