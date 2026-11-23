#!/usr/bin/env bash
# Development mode: runs the backend (127.0.0.1:8787) + traffic proxy (:8788) alongside the frontend next dev (:5173).
# The frontend /api is reverse-proxied to the backend. Stop everything with Ctrl-C.
#
# For the single-binary (embedded frontend) approach, see the "single binary" section of the README. This script is not used there.
set -euo pipefail
cd "$(dirname "$0")"

# On exit, terminate all child processes in this process group (backend + frontend).
cleanup() { kill 0 2>/dev/null || true; }
trap cleanup EXIT INT TERM

# Backend (plain go run, frontend not embedded). The concurrent work agent count is set in "System settings".
go run ./cmd/artex -addr 127.0.0.1:8787 -proxy 127.0.0.1:8788 &

# Frontend hot reload (Vite/Next dev server, /api reverse-proxied to :8787).
( cd web && npm run dev ) &

echo "[dev] Backend :8787 / proxy :8788 / frontend http://localhost:5173  (stop with Ctrl-C)"
wait
