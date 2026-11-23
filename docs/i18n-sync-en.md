# i18n Translation & Upstream Sync Guide

This fork builds on an upstream repository (`origin`) that retains Chinese and Korean source literals across backend packages, UI helpers, and shell scripts. Instead of rewriting upstream files inline, keep translations in additive boundary catalogs and track only the upstream delta on each pull.

## 1. Translation Architecture & Boundaries

Keep translations in lookup catalogs keyed by the exact original Korean or Chinese source literal, and apply them only at the user-facing output boundary (UI, HTTP/SSE DTOs, CLI):

- **Go boundary catalogs**:
  - `config/i18n.go` — shared lower-layer Chinese literals (`db`, `evidence`, `traffic`, `agent`, `notify`) via `config.TrMsg`.
  - `server/errmsg_catalog.go` — HTTP/JSON Korean error literals via `trMsg`.
  - `server/activity_msg_i18n.go` — activity stream summaries translated at read/DTO time (`activityDTO` / `activityDTOs`).
  - `selfupdate/msg_i18n.go` — updater errors and progress messages.
  - `intercept/label_i18n.go` and `intercept/msg_i18n.go` — interception action labels, verdicts, and approval cards.
  - `agent/displaymsg_i18n.go` — display filter for persisted worker terminal summaries and details.
- **Web UI (`next-intl`)**:
  - `web/messages/{en,ko,zh,es,ja}.json` — component UI strings (all five locale files must stay in key parity).
- **Web non-React & seeded-data helpers**:
  - `web/src/lib/client-msg.ts` — client-side errors thrown outside React (`api.ts`, `company-scope.ts`).
  - `web/src/lib/agent-i18n.ts` — display mapping for seeded built-in agents.
  - `web/src/lib/builtin-i18n.ts` — display mapping for seeded rules, asset notes, tool descriptions, and report headings (with source snapshots in `web/src/lib/builtin-rule-messages.json` and `web/src/lib/builtin-tool-sources.json`).

### What NOT to translate inline

- **Agent brain boundary**: LLM system/judge prompts (`agent/promptcatalog.go`, `intercept/` judge prompts) and persisted worker `result` rows read back by the planner must keep their original source text.
- **Persisted DB history**: Never translate at write time. Keep stored rows language-neutral and translate on read/DTO rendering so language switching works retroactively without altering history.
- **Wire & query sentinels**: Keep stable tokens such as `[模型]` untranslated (`intercept/msg_i18n.go`, DB `LIKE` queries, UI prefix stripping).
- **Pre-formatted strings**: For format templates (`%s`, `%d`, `%w`), translate the template *before* calling `fmt.Sprintf` or `fmt.Errorf` (avoid concatenating dynamic values before lookup, e.g., `errs("prefix" + val)`).

## 2. Git Workflow for Upstream Pulls

Enable conflict-resolution reuse once per clone:

```sh
git config rerere.enabled true
```

Keep fork translations isolated in layered commits on your branch (`main` / `mine`) rather than mixing them into unrelated changes:

1. Go boundary catalogs & minimal call-site wrappers
2. Web i18n (`web/messages/*.json`, `client-msg.ts`, etc.)
3. Shell/batch scripts (`*.sh`, `start.bat`) — keep in a separate commit (or leave untranslated if unused) so upstream script rewrites can be dropped or re-applied independently without blocking Go/Web rebases.

When syncing from upstream:

```sh
git fetch origin
git rebase origin/main
```

## 3. Post-Pull Delta Checklist

1. **Inspect only what upstream changed or added** using `scripts/check-i18n-delta.py`:

   ```sh
   # After pulling/rebasing upstream:
   python3 -I scripts/check-i18n-delta.py origin/main@{1}..origin/main --check-orphans

   # Or for uncommitted local edits:
   python3 -I scripts/check-i18n-delta.py
   ```

   This reports:
   - New or modified Go/TS string literals containing Han or Hangul that are not yet in any catalog (skipping comments, tests, catalog files, and `[模型]`).
   - Missing or empty keys in `web/messages/en.json` compared to `ko.json`/`zh.json` when `web/messages/*.json` changed.
   - Any modified `.sh` or `.bat` scripts (which cannot use Go/TS catalogs).
   - `--check-orphans`: catalog keys whose source literal was modified or removed in the diff and no longer appears in non-catalog source files.

2. **Add missing translations** to the appropriate boundary catalog (`config/i18n.go`, `server/*.go`, `intercept/msg_i18n.go`, `agent/displaymsg_i18n.go`, `web/messages/*.json`, or `web/src/lib/client-msg.ts`).

3. **Run all verification gates**:

   ```sh
   # Go build & i18n/catalog tests
   go build ./...
   go test -count=1 ./config ./selfupdate ./intercept ./agent ./server

   # Web typecheck, locale parity, unit tests, static export, and CJK leak check
   cd web
   npx tsc --noEmit
   npm run check:i18n
   npm test -- --run
   npm run build:static
   python3 -I ../scripts/check-web-cjk.py
   ```
