#!/usr/bin/env bash
set -euo pipefail

# Render assigns the PORT environment variable arbitrarily, so specify the argument so the app listens on that port
# keep the default run and skill/data paths
: "${PORT:?The PORT environment variable must be set}"
echo "[*] Starting ARTEX Server on port :$PORT"
exec ./artex -addr ":$PORT"
