# ARTEX on macOS — app bundle, Full Disk Access, and updates

This document covers the macOS-specific setup for running ARTEX from a source
checkout as a signed `/Applications/ARTEX.app`, including how to keep it working
across `git pull` updates.

It exists because macOS **TCC** (Transparency, Consent and Control) governs access
to protected data — `~/Library/Mail`, `~/Library/Messages`, Safari data, browser
profiles, and the TCC databases themselves. Two things in the default dev setup
prevented ARTEX from reading those paths:

1. **A throwaway binary path.** `go run` builds into
   `…/T/go-build*/b001/exe/artex` and ad-hoc signs it. TCC keys grants by
   executable path and code requirement, so a path that changes every launch can
   never hold a stable grant.
2. **A shell in the responsible-process chain.** `dev.sh` is a bash script, so TCC
   attributed ARTEX's file reads to
   `/opt/homebrew/Cellar/bash/…/bin/bash`. That path carries an automatic
   **deny** (`auth_value=0`, `auth_reason=5` = service policy) because a bare shell
   is not a user-facing app and is never prompted. Even `sudo` did not help: TCC
   evaluates the responsible process, not the effective UID.

The fix is a stable, signed binary at a fixed path, launched with no shell in
between, plus a one-time Full Disk Access grant you approve in System Settings.

---

## What is installed

| Path | Purpose |
|---|---|
| `<repo>/artex` | The single binary with the frontend embedded (`-tags embedui`) |
| `/Applications/ARTEX.app` | Thin signed shim that `exec`s the runtime script |
| `<repo>/scripts/macos/artex-app-run.sh` | Ensures PostgreSQL is up, then `exec`s the binary |
| `<repo>/scripts/macos/rebuild-artex.sh` | Rebuilds and re-signs after an update |
| `<repo>/scripts/macos/install-launcher.sh` | Builds/signs/installs the app bundle |

`artex-app-run.sh` uses `exec` at every hop. This is deliberate and must not be
changed to a `Terminal`-based launch: it keeps the process identity of
`ARTEX.app`, so TCC attributes file access to the app (and its signed binary)
rather than to a shell that macOS auto-denies.

The binary stays at the repository root because ARTEX resolves `data/`,
`skills/`, `config.json` and `jwt.key` relative to its own executable directory
(`config.BaseDir()`), and the self-updater swaps binaries next to the executable.
Moving the binary elsewhere would silently relocate all runtime state.

---

## First-time setup

1. **Build and sign the binary:**

   ```bash
   cd /path/to/artex-global
   ./scripts/macos/rebuild-artex.sh
   ```

   This builds with the frontend embedded, signs with your Developer ID
   Application identity under the stable identifier `org.clearpixels.artex`, and
   verifies the signature. If no Developer ID identity exists it falls back to
   ad-hoc signing and warns that the FDA grant will not persist.

2. **Install the app bundle:**

   ```bash
   ./scripts/macos/install-launcher.sh --no-notarize
   ```

   Notarisation needs App Store Connect credentials
   (`NOTARY_KEY_PATH`, `NOTARY_KEY_ID`, `NOTARY_ISSUER_ID`). Without them the app
   is signed but not notarised, which is fine for local use — Gatekeeper reports
   "Unnotarized Developer ID" but still runs it.

3. **Grant Full Disk Access — this step cannot be scripted.**

   `tccutil` can only *reset* grants, never create them, so this must be done by
   hand:

   - Open **System Settings → Privacy & Security → Full Disk Access**.
   - Click **+**, then press <kbd>⌘</kbd><kbd>⇧</kbd><kbd>G</kbd> and paste the
     absolute path to the binary:

     ```
     /path/to/artex-global/artex
     ```

   - Toggle it on.

   Add the **binary**, not the `.app` bundle: the binary is what performs the
   reads. If you also see `/opt/homebrew/Cellar/bash/…/bin/bash` listed, enable it
   too — that clears the inherited policy deny for any remaining shell hops.

4. **Launch** with `open /Applications/ARTEX.app`, or from a terminal:

   ```bash
   ./scripts/macos/artex-app-run.sh
   ```

5. **Verify the grant took effect.** In an ARTEX task, the agent should get a row
   count rather than `EPERM`:

   ```bash
   sudo -n sqlite3 "/Library/Application Support/com.apple.TCC/TCC.db" \
     "select count(*) from access;"
   ```

   A number (not "authorization denied") means Full Disk Access is working. If it
   still fails, confirm the running process is the signed binary:

   ```bash
   pgrep -f 'artex-global/artex'          # should print a PID
   codesign -dv "$(pgrep -f 'artex-global/artex' | head -1)" 2>&1 | head -5
   ```

   `Identifier=org.clearpixels.artex` and a Developer ID authority mean TCC will
   match your grant.

---

## Updating after `git pull`

The app bundle is a shim, so it never needs reinstalling after a source update.
The only work is rebuilding and re-signing the binary.

**Backend-only changes** (the common case) take seconds:

```bash
cd /path/to/artex-global
git pull --ff-only
./scripts/macos/rebuild-artex.sh --restart
```

`rebuild-artex.sh` rebuilds the frontend **only** when files under `web/` are
newer than the exported frontend in `server/webui/dist`, so a backend-only pull
skips `npm ci` and the Next build entirely.

**Frontend changes** (anything under `web/`) are detected automatically. To force
it, or to skip it deliberately:

```bash
./scripts/macos/rebuild-artex.sh --with-frontend   # force frontend rebuild
./scripts/macos/rebuild-artex.sh --skip-frontend   # backend only
```

If you have uncommitted changes and do not want to pull:

```bash
./scripts/macos/rebuild-artex.sh --restart
```

### Why the Full Disk Access grant survives updates

The signing identity is fixed: identifier `org.clearpixels.artex` and the same
Developer ID certificate. TCC stores a **Designated Requirement**, which for this
setup resolves to:

```
identifier "org.clearpixels.artex" and anchor apple generic and … certificate
leaf[subject.OU] = <YOUR_TEAM_ID>
```

That requirement is identical across rebuilds, so the grant you approved once
continues to match the new binary. Verify it after any rebuild:

```bash
codesign -d -r- artex
```

If the identity or identifier ever changes, TCC will treat the new binary as a
different program and you must re-approve Full Disk Access. The rebuild script
warns about this at the end.

**Do not** sign with `--sign -` (ad-hoc) if you want the grant to persist: ad-hoc
signatures have no stable identity, so every rebuild looks like a new program.

---

## Troubleshooting

**"authorization denied" or `EPERM` reading TCC.db, even with `sudo`.**
TCC evaluates the *responsible process*, not the effective user. Check the
attribution in the log:

```bash
sudo log show --last 5m --predicate 'subsystem == "com.apple.TCC"' --style compact \
  | grep -iE 'artex|deny'
```

If `responsible_path` names a shell (`/bin/bash`, homebrew bash) rather than
`/path/to/artex-global/artex`, something in the launch chain is still using a
shell instead of `exec`. Confirm the process tree:

```bash
PID=$(pgrep -f 'artex-global/artex' | head -1)
ps -p "$PID" -o pid=,ppid=,comm=
```

PPID should be `1` (launched by launchd via the app). If it is a shell PID, the
app bundle or runtime script is not using `exec`.

**The app opens but the UI is unreachable.**
Check the log, which the runtime script writes:

```bash
tail -40 ~/Library/Logs/ARTEX/artex.log
```

**Docker/PostgreSQL is not found.**
The runtime script prepends `/opt/homebrew/bin` and `/usr/local/bin` to `PATH`
because Finder-launched apps get only `/usr/bin:/bin:/usr/sbin:/sbin`. If Docker
Desktop is installed elsewhere, start it manually and relaunch.

**A port is already in use.**
The runtime script detects an existing listener on 8787 and just opens the UI
instead of starting a second stack. Stop the old stack first if you want a clean
restart.

**Signature rejected after a rebuild.**
Run `codesign --verify --strict --verbose=2 artex`. If it fails, the build was
modified after signing; re-run `rebuild-artex.sh`.

---

## Uninstalling

```bash
./scripts/macos/install-launcher.sh --uninstall
```

Then remove the binary and, if you wish, the Full Disk Access entry in System
Settings. Runtime state (`data/`, `jwt.key`) stays in the checkout.

---

## Environment overrides

`artex-app-run.sh`:

| Variable | Default | Purpose |
|---|---|---|
| `ARTEX_APP_BACKEND_PORT` | `8787` | Backend and UI port |
| `ARTEX_APP_PROXY_PORT` | `8788` | Traffic recording proxy |
| `ARTEX_DEV_PG_PORT` | `5433` | PostgreSQL port |
| `ARTEX_APP_PG_CONTAINER` | `artex-dev-postgres` | Docker container name |
| `ARTEX_APP_PG_VOLUME` | `artex-dev-pgdata` | Docker volume name |
| `ARTEX_APP_NO_BROWSER` | `0` | Set `1` to skip opening the browser |
| `ARTEX_APP_LOG` | `~/Library/Logs/ARTEX/artex.log` | Log destination |

`rebuild-artex.sh`:

| Variable | Purpose |
|---|---|
| `ARTEX_CODESIGN_IDENTITY` | Signing identity (default: first Developer ID Application) |
