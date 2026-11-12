#!/usr/bin/env bash
set -euo pipefail
GO_BIN="${GO_BIN:-go}"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 "$GO_BIN" build -trimpath -ldflags='-s -w -H=windowsgui' -o OpenWave-7.exe .
