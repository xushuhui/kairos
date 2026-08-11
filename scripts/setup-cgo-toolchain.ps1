<#
.SYNOPSIS
    One-time dev machine setup: installs the MinGW-w64 gcc toolchain (via MSYS2)
    and enables cgo, which this repo needs to build fyne.io/fyne/v2 (go-gl/gl)
    and github.com/k2-fsa/sherpa-onnx-go-windows.

.DESCRIPTION
    Idempotent — safe to re-run. Skips any step whose target is already present.
    Does NOT touch Go itself, ffmpeg, CUDA, or model files; see README.md
    "Requirements" for those.

.NOTES
    Run in an elevated PowerShell (winget install may prompt for UAC).
#>

$ErrorActionPreference = "Stop"

function Test-CommandExists($name) {
    return [bool](Get-Command $name -ErrorAction SilentlyContinue)
}

# 1. gcc already usable? Nothing to do.
if (Test-CommandExists "gcc") {
    Write-Host "[skip] gcc already on PATH: $(gcc --version | Select-Object -First 1)"
} else {
    # 2. MSYS2 installed?
    $msys2Root = "C:\msys64"
    if (-not (Test-Path "$msys2Root\usr\bin\bash.exe")) {
        if (-not (Test-CommandExists "winget")) {
            throw "winget not found. Install MSYS2 manually from https://www.msys2.org/ then re-run this script."
        }
        Write-Host "[install] MSYS2 via winget..."
        winget install --id MSYS2.MSYS2 -e --accept-source-agreements --accept-package-agreements
        if (-not (Test-Path "$msys2Root\usr\bin\bash.exe")) {
            throw "MSYS2 install did not land at $msys2Root as expected. Check winget output above."
        }
    } else {
        Write-Host "[skip] MSYS2 already installed at $msys2Root"
    }

    # 3. Install the UCRT64 gcc package.
    #    Uses `pacman -Sy` (sync db + install one package) instead of the full
    #    `-Syu` system upgrade dance, which requires closing/reopening the
    #    MSYS2 shell mid-update — not automatable from a single script call.
    Write-Host "[install] mingw-w64-ucrt-x86_64-gcc via pacman..."
    & "$msys2Root\usr\bin\bash.exe" -lc "pacman -Sy --noconfirm mingw-w64-ucrt-x86_64-gcc"

    $gccPath = "$msys2Root\ucrt64\bin"
    if (-not (Test-Path "$gccPath\gcc.exe")) {
        throw "gcc.exe not found at $gccPath after install. Check pacman output above."
    }

    # 4. Add to the user PATH permanently (persists across new shells).
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$gccPath*") {
        Write-Host "[path] Adding $gccPath to user PATH..."
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$gccPath", "User")
    } else {
        Write-Host "[skip] $gccPath already on user PATH"
    }
    $env:Path = "$env:Path;$gccPath"
}

# 5. Enable cgo for the current user's Go environment.
$cgoEnabled = go env CGO_ENABLED
if ($cgoEnabled -ne "1") {
    Write-Host "[go env] Setting CGO_ENABLED=1..."
    go env -w CGO_ENABLED=1
} else {
    Write-Host "[skip] CGO_ENABLED already 1"
}

Write-Host ""
Write-Host "Done. Open a NEW terminal (to pick up the PATH change), then verify:"
Write-Host "  gcc --version"
Write-Host "  go env CGO_ENABLED"
Write-Host "  go build ./..."
