# Privacy & Data-Handling Review — ARTEX global edition

Scope: privacy and data-protection review of the repository at commit `071f050`
(working tree). Review only — no code was changed.

Reviewed against a privacy/data-protection lens: personal data (PII) at rest,
data egress to third parties, secrets handling, retention, and logging. This is
not a general application-security audit (that is covered in
[`../SECURITY.md`](../SECURITY.md)).

## Context

ARTEX is an autonomous penetration-testing tool. It ingests third-party target
data — HTTP response bodies, headers, cookies, bearer tokens, and potentially
personal data — and forwards that data to LLM providers. The dominant privacy
risks are therefore:

1. **PII at rest with no retention policy**, and
2. **Data egress to third parties** (LLM provider, optional search backend).

Classic web-app authorization bugs are largely absent here because ARTEX is a
single-operator tool (`ARTEX` account) by design.

## Findings

| # | Severity | Issue | Location |
|---|---|---|---|
| 1 | **High** | Traffic capture is **full plaintext, no redaction, no retention/TTL**. Every request/response header (including `Authorization`, `Cookie`, `Set-Cookie`) and full body is written to disk and SQLite; large bodies spill to blob files at mode `0644`. Nothing ages out — deletion is manual only. Target PII persists indefinitely. | `traffic/traffic.go:4` ("Full capture, plaintext (no redaction)"), `:450-451` (headers written verbatim), `:552` (blob `0644`); no purge job anywhere |
| 2 | **High** | **Transcripts are always-on and unbounded.** Full-fidelity LLM conversations (user/assistant/tool messages, i.e. target response bodies and PII) are appended to `data/transcripts/*.jsonl` at mode `0644`, independent of the `llm_record` toggle. They are deleted only when the owning task is deleted. | `server/server.go:573`; `agent/{worker,planner,mainagent,chat}.go` all receive the transcript store; norma `transcript.Append` uses `0644` |
| 3 | **High** | **All target data egresses to a third-party LLM provider** (Anthropic/OpenAI/custom endpoint). Scope, assets, findings, and raw tool output including PII go to the configured provider. This is not prominently disclosed; the in-app Terms Art.5 ("we do not collect or upload your usage data") refers to the *authors*, not the operator's provider, and is easy to misread. | `agent/provider.go`; `llmrec`; `web/messages/en.json` (`auth.terms.a5Body`) |
| 4 | **Medium** | **LLM request/response bodies** are stored in Postgres (`llm_records`). Recording is gated off by default (good), but rows are kept until per-task manual deletion; the `llm_usage` metering ledger is never deleted (by design). | `db/commands.go:172`; `server/manager.go:429`; `db/tasks.go:639` |
| 5 | **Medium** | **Evidence snapshots** retain request/response headers and bodies indefinitely while bound to a finding. GC only reaps *unreferenced* snapshots after a 24-hour grace period. Correct by design, but PII lives as long as the finding does. | `evidence/store.go:62,348-415` |
| 6 | **Medium** | **MCP `env` is returned raw** from the API and stored plaintext — unlike LLM profiles (`APIKey json:"-"`) and notification channels (`MaskedValue`). It can contain `Authorization: Bearer …`. Inconsistent masking. | `db/config.go:815`; `server/server_mgmt.go:914` (`pgListMCP` returns rows as-is) vs `notify/mask.go` |
| 7 | **Medium** | **JWT stored in `localStorage` and in a cookie without `Secure`/`HttpOnly`.** The token is XSS-readable, and the cookie is not `Secure`, so it can travel over plain HTTP when not behind TLS. TTL is 7 days. | `web/src/lib/auth.ts:23` (`SameSite=Lax`, no `Secure`); `web/src/lib/api.ts:123` |
| 8 | **Medium** | **`server_logs` has no retention** and persists indefinitely. Some lines log full URLs (which may carry query-string tokens/PII), and `[llm-test]` logs a **response body**. | `server/logsink.go`; `agent/provider.go:435`; no `DELETE FROM server_logs` anywhere |
| 9 | **Low** | **No encryption at rest.** Traffic blobs and transcripts use mode `0644` (world-readable on a shared multi-user host); Postgres stores plaintext. Evidence files are `0600`/`0700` (good). | `traffic/traffic.go:552`; norma transcript `0644` |
| 10 | **Low** | **Update check** contacts `api.github.com` when the UI loads (version/IP disclosure, no payload). **Web search** (default off) sends queries to Brave/Tavily/DDG. Both are effectively opt-in but not documented in privacy terms. | `server/update.go:217`; `server/manager.go:435`; `web/src/app/(main)/_components/update-badge.tsx:29` |
| 11 | **Low** | **No "data handling" documentation section.** The legal notice mentions the Korean Personal Information Protection Act (PIPA) but there is no "what is stored where, for how long, and how to delete it" statement — a gap for PIPA/GDPR storage-limitation and transparency duties. | `README.md` "Read first" section; `docs/` has no privacy document |

## What is done well

- Traffic capture and LLM body recording are **opt-in (default off)** —
  `server/manager.go:427,429`.
- Secrets are gitignored and never committed (`jwt.key`, `.env`, `*.env`,
  `.secrets/`); no hardcoded API keys were found.
- `jwt.key` was moved out of the browsable workspace to the key directory, mode
  `0600` (`server/auth.go:72-105`).
- Passwords are bcrypt-hashed; login is rate-limited with backoff; comparison is
  constant-time; no username oracle (`server/auth.go:658-700`).
- LLM profile keys are never serialized (`json:"-"` plus `api_key_hint`);
  notification credentials are masked with the `__masked__` prefix
  (`notify/mask.go`).
- Notification delivery has an SSRF guard (dial-layer, opt-in local override) and
  redacts URL credentials from error messages (`notify/http.go:205-250`).
- CORS is restricted to an allowlist rather than `*`
  (`server/server.go:4024-4080`).
- Self-update is pinned to HTTPS, a GitHub host allowlist, and SHA-256
  verification (`selfupdate/github.go`, `selfupdate/stage.go`).
- Workspace path traversal is confined to the root
  (`server/workspace.go:42-58`).
- The web UI has no third-party analytics, telemetry, fonts, or CDN dependencies
  (same-origin only, system fonts).

## Recommended actions (priority order)

1. **Add a retention policy** for traffic, transcripts, and `server_logs`
   (configurable TTL plus a scheduled purge), or at minimum document that
   operators must purge manually. This is the single largest privacy gap for
   PIPA/GDPR storage limitation.
2. **Mask MCP `env`** the way notification channels and LLM profiles are masked.
3. **Add `Secure` to the auth cookie** (and consider `HttpOnly` via a
   server-set cookie) — `web/src/lib/auth.ts:23`.
4. **Trim the `[llm-test]` response-body logging** or gate it behind a debug
   flag (`agent/provider.go:435`).
5. **Add a short "Data & privacy" README section**: what is stored
   (traffic/transcripts/evidence/LLM records), where, for how long, how to
   delete it, and a clear note that *scan data is sent to your configured LLM
   provider and optionally to search providers* — reconciling the Terms Art.5
   wording.
6. **Tighten file modes** on traffic blobs and transcripts to `0600` if the host
   is shared.

## Coverage

Categories reviewed: secrets management, input validation (path traversal,
uploads), authentication/authorization, injection/escaping, sensitive data at
rest, data egress, retention, and logging.

Not applicable / out of scope: payment features (none); multi-tenant
authorization (single-user model by design).
