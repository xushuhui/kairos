#!/usr/bin/env bash
# scripts/download-models.sh — macOS/Linux dev-loop equivalent of
# scripts/download-models.ps1. Downloads the Paraformer-large +
# Silero-VAD + punctuation ONNX model files internal/asr expects, laid out
# at <out_dir>/models/{paraformer,vad,punctuation}/ — the exact structure
# modelDirPath() (cmd/kairos/transcriber_supported.go) looks for next to
# the built binary.
#
# These are real binary model artifacts (~500 MB total) fetched from the
# official k2-fsa/sherpa-onnx GitHub releases — not vendored in this repo
# (see packaging/README.md). Idempotent: skips any file already present at
# its expected path/name instead of re-downloading.
#
# Usage: scripts/download-models.sh [out_dir]   (defaults to repo root)
#
# FunASR Model License — commercial use requires attributing
# Alibaba/FunAudioLLM (see README.md's Requirements section).

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/.." && pwd)"
out_dir="${1:-$root}"
models_dir="$out_dir/models"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

download_if_missing() {
  local url="$1" dest="$2"
  if [ -f "$dest" ]; then
    echo "[skip] $dest already exists"
    return
  fi
  echo "[download] $url"
  curl -sSL -o "$dest" "$url"
}

# 1. Paraformer-large (model.int8.onnx + tokens.txt) — 2023-09-14 release,
#    the version map.md's decision log recommends (character-level
#    timestamp support, matches paraformerModelFile/paraformerTokensFile
#    in internal/asr/merge.go).
echo "==> Paraformer-large"
paraformer_dir="$models_dir/paraformer"
mkdir -p "$paraformer_dir"
if [ -f "$paraformer_dir/model.int8.onnx" ] && [ -f "$paraformer_dir/tokens.txt" ]; then
  echo "[skip] paraformer model files already present"
else
  archive="$tmp_dir/paraformer.tar.bz2"
  download_if_missing "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-2023-09-14.tar.bz2" "$archive"
  tar xjf "$archive" -C "$tmp_dir"
  cp "$tmp_dir/sherpa-onnx-paraformer-zh-2023-09-14/model.int8.onnx" "$paraformer_dir/"
  cp "$tmp_dir/sherpa-onnx-paraformer-zh-2023-09-14/tokens.txt" "$paraformer_dir/"
fi

# 2. Silero VAD — single file, no archive.
echo "==> Silero VAD"
vad_dir="$models_dir/vad"
mkdir -p "$vad_dir"
download_if_missing "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx" "$vad_dir/silero_vad.onnx"

# 3. Punctuation restoration — code hardcodes punctuationModelFile =
#    "punctuation/model.onnx", so this must be the FP32 archive, not the
#    -int8 variant (different internal filename, model.int8.onnx).
echo "==> Punctuation restoration"
punct_dir="$models_dir/punctuation"
mkdir -p "$punct_dir"
if [ -f "$punct_dir/model.onnx" ]; then
  echo "[skip] punctuation model already present"
else
  archive="$tmp_dir/punct.tar.bz2"
  download_if_missing "https://github.com/k2-fsa/sherpa-onnx/releases/download/punctuation-models/sherpa-onnx-punct-ct-transformer-zh-en-vocab272727-2024-04-12.tar.bz2" "$archive"
  tar xjf "$archive" -C "$tmp_dir"
  cp "$tmp_dir/sherpa-onnx-punct-ct-transformer-zh-en-vocab272727-2024-04-12/model.onnx" "$punct_dir/"
fi

echo
echo "Done. Models at: $models_dir"
echo "FunASR Model License: commercial use must attribute Alibaba/FunAudioLLM (see README.md)."
