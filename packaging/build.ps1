# packaging/build.ps1 — assembles the dist/ staging directory and invokes
# go-msi. MUST run on Windows (go-msi itself requires WiX >= 3.10 on PATH,
# neither of which exist on this repo's macOS dev machine — this script is
# written but genuinely unverified, see packaging/README.md).
#
# Usage: pwsh ./packaging/build.ps1 -Version 0.1.0
#   -FfmpegDir   directory containing ffmpeg.exe/ffprobe.exe            (required)
#   -ModelsDir   directory containing the Paraformer/Silero-VAD/        (required)
#                punctuation model files, copied to dist/models/
#   -CudaDir     directory containing the CUDA/cuDNN runtime DLLs       (required)
#                listed in wix.json's files.items
#   -Version     product version passed to go-msi                      (required)

param(
    [Parameter(Mandatory = $true)][string]$FfmpegDir,
    [Parameter(Mandatory = $true)][string]$ModelsDir,
    [Parameter(Mandatory = $true)][string]$CudaDir,
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

Write-Host "==> Copying CUDA/cuDNN runtime DLLs"
# Filenames come from wix.json's files.items (single source of truth — no
# second hardcoded list to keep in sync). See packaging/README.md for the
# version-pinning caveat before changing wix.json's DLL entries.
$manifest = Get-Content (Join-Path $root "packaging/wix.json") | ConvertFrom-Json
$cudaDlls = $manifest.files.items |
    Where-Object { $_ -like "dist/*.dll" } |
    ForEach-Object { Split-Path -Leaf $_ }
foreach ($dll in $cudaDlls) {
    Copy-Item (Join-Path $CudaDir $dll) $dist
}

Write-Host "==> NVIDIA redistribution notice"
# Required by wix.json's files.items — NVIDIA permits redistributing these
# runtime DLLs under its own license terms; this file must contain that
# notice text (05-deployment-cicd.md). Not fabricated here — fill in from
# NVIDIA's actual redistribution EULA before shipping.
if (-not (Test-Path (Join-Path $CudaDir "NVIDIA_REDIST_LICENSE.txt"))) {
    Write-Warning "NVIDIA_REDIST_LICENSE.txt not found in -CudaDir — MSI build will fail until it's supplied."
} else {
    Copy-Item (Join-Path $CudaDir "NVIDIA_REDIST_LICENSE.txt") $dist
}

Write-Host "==> Running go-msi"
Push-Location (Join-Path $root "packaging")
go-msi set-guid --path wix.json  # no-op after the first run; guids persist in wix.json once generated
go-msi make --path wix.json --msi "kairos-$Version.msi" --version $Version --license LICENSE.rtf
Pop-Location

Write-Host "==> Done: packaging/kairos-$Version.msi"
