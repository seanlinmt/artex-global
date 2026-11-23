#!/bin/sh
# ARTEX daemon start script (Linux / macOS / Docker ENTRYPOINT)
#
# Usage:
#   ./start.sh                       Run in the foreground (stop with Ctrl-C)
#   nohup ./start.sh >artex.log 2>&1 &   Run as a background daemon
#   ./start.sh -addr :9000           Extra arguments are passed through to artex
#
# This script does only one thing. It runs artex and, when the process exits, decides whether to relaunch based on the exit code.
#
#   0      User stopped normally     → end the loop
#   75     Program requested restart → relaunch immediately ("one-click update" or "rollback" pressed in the UI)
#   other  Abnormal exit            → relaunch after backoff (1→2→4…up to 60 seconds)
#
# Download, SHA256 checksum verification, and version swapping are deliberately not done here. That logic would have to be
# written twice, in sh and bat, and it is precisely the part that must never be wrong. If it is swapped to a binary that does
# not run, this script faithfully keeps relaunching it, and the user has no choice but to log in and recover manually. So all
# verification and swapping lives in Go (the selfupdate package), letting artex handle it itself at startup; the script stays simple.
set -u

cd "$(dirname "$0")" || exit 1

BIN=./artex
[ -x "$BIN" ] || { echo "[artex] Executable not found: $BIN" >&2; exit 1; }

RESTART_CODE=75
MAX_DELAY=60

child=0
stopping=0

# Forward the stop signal to the artex process.
#
# In Docker this is essential. docker stop sends SIGTERM only to PID 1 (i.e. this script), not to child processes.
# Without forwarding, artex never receives the signal, cannot shut down gracefully, and is force-killed by SIGKILL
# 10 seconds later, cutting off in-flight work.
forward() {
	stopping=1
	if [ "$child" -ne 0 ]; then
		kill -TERM "$child" 2>/dev/null || true
	fi
}
trap forward INT TERM

delay=1
while :; do
	"$BIN" "$@" &
	child=$!

	# A signal interrupts wait, making it return a value greater than 128. The child process is still shutting down
	# gracefully at that point, so a second wait is needed to get the real exit code.
	wait "$child"
	code=$?
	if [ "$code" -gt 128 ]; then
		wait "$child"
		code=$?
	fi
	child=0

	if [ "$stopping" -eq 1 ]; then
		echo "[artex] Stopped"
		exit 0
	fi

	case "$code" in
		0)
			echo "[artex] Normal exit"
			exit 0
			;;
		"$RESTART_CODE")
			# An update/rollback is ready. Relaunching lets artex complete the version swap at startup (see selfupdate.Bootstrap).
			echo "[artex] Restart requested (applying new version)…"
			delay=1
			;;
		*)
			echo "[artex] Abnormal exit (code=$code), restarting in ${delay}s" >&2
			sleep "$delay"
			delay=$((delay * 2))
			[ "$delay" -gt "$MAX_DELAY" ] && delay=$MAX_DELAY
			;;
	esac
done
