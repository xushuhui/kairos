<#
.SYNOPSIS
    Downloads the Paraformer-large + Silero-VAD + punctuation ONNX model
    files internal/asr/paraformer_windows.go expects, and lays them out at
    <OutDir>\models\{paraformer,vad,punctuation}\ — the exact structure
    modelDirPath() (cmd/kairos/transcriber_windows.go) looks for next to
    kairos.exe.

.DESCRIPTION
    These are real binary model artifacts (~500 MB total) fetched from the
    official k2-fsa/sherpa-onnx GitHub releases — not vendored in this repo
    (see packaging/README.md). Idempotent: skips any file already present
    at its expected path/name instead of re-downloading.

.PARAMETER OutDir
    Directory that receives a `models\` subdirectory. Defaults to `bin\`
    (matching scripts/build-dev.ps1's build output), so on a fresh dev box
    you can run this once and scripts/build-dev.ps1 afterwards and have a
    runnable kairos.exe with everything next to it.

.NOTES
    FunASR Model License — commercial use requires attributing
    Alibaba/FunAudioLLM (see README.md's Requirements section).
#>

param(
    [string]$OutDir = (Join-Path (Split-Path -Parent $PSScriptRoot) "bin")
)

$ErrorActionPreference = "Stop"

$modelsDir = Join-Path $OutDir "models"
$tmpDir = Join-Path $env:TEMP "kairos-model-dl"
New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null

function Get-IfMissing($url, $dest) {
    if (Test-Path $dest) {
        Write-Host "[skip] $dest already exists"
        return
    }
    Write-Host "[download] $url"
    Invoke-WebRequest -Uri $url -OutFile $dest
}

# 1. Paraformer-large (model.int8.onnx + tokens.txt) — 2023-09-14 release,
#    the version map.md's decision log already recommends (character-level
#    timestamp support, matches paraformerModelFile/paraformerTokensFile in
#    internal/asr/paraformer_windows.go).
Write-Host "==> Paraformer-large"
$paraformerDir = Join-Path $modelsDir "paraformer"
New-Item -ItemType Directory -Path $paraformerDir -Force | Out-Null
$paraformerModel = Join-Path $paraformerDir "model.int8.onnx"
$paraformerTokens = Join-Path $paraformerDir "tokens.txt"
if ((Test-Path $paraformerModel) -and (Test-Path $paraformerTokens)) {
    Write-Host "[skip] paraformer model files already present"
} else {
    $archive = Join-Path $tmpDir "paraformer.tar.bz2"
    Get-IfMissing "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-2023-09-14.tar.bz2" $archive
    tar xf $archive -C $tmpDir
    Copy-Item (Join-Path $tmpDir "sherpa-onnx-paraformer-zh-2023-09-14\model.int8.onnx") $paraformerDir -Force
    Copy-Item (Join-Path $tmpDir "sherpa-onnx-paraformer-zh-2023-09-14\tokens.txt") $paraformerDir -Force
}

# 2. Silero VAD — single file, no archive.
Write-Host "==> Silero VAD"
$vadDir = Join-Path $modelsDir "vad"
New-Item -ItemType Directory -Path $vadDir -Force | Out-Null
Get-IfMissing "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx" (Join-Path $vadDir "silero_vad.onnx")

# 3. Punctuation restoration — code hardcodes punctuationModelFile =
#    "punctuation/model.onnx", so this must be the FP32 archive, not the
#    -int8 variant (different internal filename, model.int8.onnx).
Write-Host "==> Punctuation restoration"
$punctDir = Join-Path $modelsDir "punctuation"
New-Item -ItemType Directory -Path $punctDir -Force | Out-Null
$punctModel = Join-Path $punctDir "model.onnx"
if (Test-Path $punctModel) {
    Write-Host "[skip] punctuation model already present"
} else {
    $archive = Join-Path $tmpDir "punct.tar.bz2"
    Get-IfMissing "https://github.com/k2-fsa/sherpa-onnx/releases/download/punctuation-models/sherpa-onnx-punct-ct-transformer-zh-en-vocab272727-2024-04-12.tar.bz2" $archive
    tar xf $archive -C $tmpDir
    Copy-Item (Join-Path $tmpDir "sherpa-onnx-punct-ct-transformer-zh-en-vocab272727-2024-04-12\model.onnx") $punctDir -Force
}

Remove-Item $tmpDir -Recurse -Force -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "Done. Models at: $modelsDir"
Write-Host "FunASR Model License: commercial use must attribute Alibaba/FunAudioLLM (see README.md)."
