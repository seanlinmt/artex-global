# ARTEX on Linux — install, service, exposure, and updates

This document covers running ARTEX on a Linux host: the supported deployment
paths, first-run setup, running it as a long-lived `systemd` service, controlling
which interface the UI is published on, putting HTTPS in front of it, and
updating without losing data.

It is the Linux counterpart to [`macos-install-en.md`](macos-install-en.md). That
document exists because macOS **TCC** gates access to protected paths, which needs
a signed app bundle and a Full Disk Access grant. Linux has no equivalent
per-app consent layer, so there is nothing to sign or approve here — but Linux
brings its own sharp edges: `systemd` unit semantics, firewall interaction with
Docker's port publishing, and running the agent's tool calls with enough
privilege for `nmap` and similar probes.

> **Read the security warning in the [README](../README.md) first.** ARTEX is an
> autonomous offensive tool. Run it only against targets you own or are
> authorized in writing to test, and prefer isolated lab targets (OWASP Juice
> Shop, DVWA) while you learn it.

---

## Supported deployments

| Method | Best for | Database |
|---|---|---|
| Docker Compose (`docker-compose.yml`) | A self-contained stack, including the toolchain the agents call | `postgres` service from the same compose file |
| `install.sh` | Interactive setup that installs Docker for you or compiles locally | Existing PostgreSQL, or a container started for you |
| Precompiled release (`build.sh --release` artifacts, or the project's Releases page) | A host without a Go/Node toolchain | Existing PostgreSQL you provide |
| Single binary built from source | Development and auditing the build | Existing PostgreSQL you provide |

**Prerequisites**

- Docker path: Docker Engine and the Compose plugin. `install.sh` can install
  Docker on Linux via `get.docker.com`; afterwards log out and back in so the
  `docker` group membership applies, or run compose with `sudo`.
- Source path: Go **1.26 or newer**, Node.js **22** with npm, and `rsync`.
- Every path needs PostgreSQL reachable from the host or container, and an LLM
  (`ANTHROPIC_API_KEY` or `OPENAI_API_KEY`, or an OpenAI-compatible endpoint —
  see [Configuration](../README.md#configuration)).

The binary resolves `config.json`, `data/` and `skills/` **relative to the
executable's directory** (or the working directory under `go run`). Keep the
binary at a fixed path and keep its runtime state next to it; see
[Running as a systemd service](#running-as-a-systemd-service-bare-metal).

---

## Quick start with Docker Compose

```bash
git clone https://github.com/jiwoochris/artex-global.git
cd artex-global
cp .env.example .env            # set POSTGRES_PASSWORD; the LLM key is optional
docker compose up -d
docker compose logs artex | grep -F '[auth]'   # the setup token for /setup
```

Then open `http://localhost:8787` and create the admin password at `/setup` with
that token. The compose file publishes `8787` on `${ARTEX_BIND}` (default
`127.0.0.1`) and keeps the recording proxy on `8788` bound to loopback only,
because it MITMs TLS with its own CA and is consumed by the agents inside the
container — an external browser never needs to reach it.

`./data` and `./skills` are bind-mounted from the project directory, so they
survive `docker compose down` and container recreation.

If the image pull fails, check that `ARTEX_TAG` in `.env` names a published tag
(`latest`, or a `v*` release tag) and that the registry is reachable.

---

## Interactive install script

```bash
./install.sh
```

The script detects Docker (offering to install it on Linux), then lets you pick:

- **① Full Docker** — writes `.env` with a Postgres password you enter or accept
  randomly, then runs `docker compose up -d`.
- **② Local compile and run** — connects to an existing PostgreSQL or starts one
  in a container, writes `config.json`, builds the frontend, compiles the
  single binary with the frontend embedded, and starts it in the foreground.

Both end at `http://localhost:8787` with the setup token printed in the console.

---

## Precompiled release

`./build.sh --release` produces `dist/artex-<version>-linux-<arch>.zip` for
`amd64` and `arm64`, plus a `SHA256SUMS` file. Each archive contains the
binary, `start.sh`, `skills/`, `config.example.json`, and the README.

```bash
sha256sum -c SHA256SUMS            # verify before you run it
unzip artex-<version>-linux-amd64.zip
cd artex-<version>-linux-amd64
cp config.example.json config.json # fill in the database block
export ARTEX_PG_DSN='postgres://artex:pass@127.0.0.1:5432/artex?sslmode=disable'
./start.sh                         # → http://localhost:8787
```

Launch with `start.sh`, not `./artex` directly. `start.sh` is the supervisor the
UI's one-click update relies on: it relaunches the process on exit code `75`
after a version swap, and backs off on abnormal exits. Running the binary
directly means an update completes but nothing restarts it.

---

## Build from source

```bash
# 1) static frontend export (Node 22)
cd web && npm ci && npm run build:static && cd ..

# 2) copy the export into the embed directory without nesting
rm -rf server/webui/dist && mkdir -p server/webui/dist && cp -a web/out/. server/webui/dist/

# 3) compile with the frontend embedded
CGO_ENABLED=0 go build -tags embedui -trimpath -o artex ./cmd/artex

./start.sh
```

`-tags embedui` is what embeds the frontend; without it the binary serves the API
only and you must run `web/` separately (`./dev.sh` runs backend, proxy, and the
Next dev server together).

> If your shell exports `NODE_ENV=production`, `npm ci` skips devDependencies and
> the build fails with `Cannot find module '@tailwindcss/postcss'`. Use
> `npm ci --include=dev` in that case.

`./build.sh` also accepts `--target linux/amd64` for a single cross-compiled
artifact. UPX compression is off by default because UPX-packed binaries can be
incompatible with some kernels, virtualization layers, and security policies;
opt in with `--upx` only after confirming it runs on your target hosts.

---

## Running as a systemd service (bare metal)

Use this when you run the binary directly rather than under Docker. Lay out a
fixed directory, run it as a dedicated unprivileged user, and let `start.sh` own
the restart-on-update protocol.

```bash
sudo useradd --system --create-home --home-dir /opt/artex --shell /usr/sbin/nologin artex
sudo mkdir -p /opt/artex /etc/artex
sudo cp -a artex start.sh skills /opt/artex/
sudo cp config.example.json /opt/artex/config.json     # then edit the database block
sudo chown -R artex:artex /opt/artex
sudo install -m 0640 -o root -g artex /dev/null /etc/artex/artex.env
```

`/etc/artex/artex.env` (readable only by root and the service group):

```sh
ARTEX_PG_DSN=postgres://artex:change-me@127.0.0.1:5432/artex?sslmode=disable
ANTHROPIC_API_KEY=sk-...
ARTEX_LANG=en
# Optional: pin the first-run setup token (systemd does not strip inline comments,
# so keep comments on their own line).
ARTEX_SETUP_TOKEN=
# Optional: set the admin password at startup and skip /setup entirely (8+ chars).
ARTEX_ADMIN_PASSWORD=
```

`/etc/systemd/system/artex.service`:

```ini
[Unit]
Description=ARTEX autonomous penetration-testing backend
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=artex
Group=artex
WorkingDirectory=/opt/artex
EnvironmentFile=/etc/artex/artex.env
ExecStart=/opt/artex/start.sh
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
# SIGTERM goes to start.sh (PID of ExecStart), which forwards it to artex so it can
# shut down gracefully; the remaining children are then killed.
KillMode=mixed
# Raw-socket capability for nmap's default SYN scan, without running as root.
AmbientCapabilities=CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_RAW

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now artex
sudo journalctl -u artex -f        # startup banner, config path, setup token
```

Notes that matter for this setup:

- **`start.sh` already supervises the process.** It relaunches on exit code `75`
  (the update/rollback protocol) and backs off on crashes, so `systemd` never
  sees `75`; `Restart=on-failure` only covers the script itself dying. Do not
  replace `ExecStart` with `./artex`, or the UI's one-click update stops working.
- **The self-updater writes next to the executable.** It stages `artex.new`,
  keeps `artex.old`, and swaps the binary in its own directory, so the service
  user must own `/opt/artex`. The same applies to `data/` and `skills/`.
- **Privilege for tool calls.** The agents execute real tools over Bash. `nmap`'s
  default SYN scan needs raw sockets; `AmbientCapabilities=CAP_NET_RAW` covers
  that without root. If you instead grant the service user `sudo`, do **not** set
  `NoNewPrivileges=yes`, which blocks setuid binaries.
- **Database migrations are automatic.** Every start re-runs the embedded schema
  idempotently, so there is no separate migrate step.
- **Ports below 1024.** The default `8787` needs no privilege; only if you bind
  `:80`/`:443` directly (not via a reverse proxy) add `CAP_NET_BIND_SERVICE`.

`./start.sh -addr 127.0.0.1:8787 -proxy 127.0.0.1:8788` passes extra flags
through; put them on the `ExecStart` line rather than in the environment file.

---

## First-run protection

There is a single admin account (`ARTEX`) whose password is created on first
visit at `/setup`. Three protections keep a not-yet-initialised instance from
being claimed by whoever reaches it first:

- **Setup token.** With no password set, startup logs one line
  `[auth] … Setup token: …` and `/setup` (or `POST /api/auth/init` with
  `setup_token`) only accepts a matching value; wrong attempts are rate-limited.
  For unattended deployments pin it with `ARTEX_SETUP_TOKEN`, or skip `/setup`
  entirely with `ARTEX_ADMIN_PASSWORD` (8+ characters).
- **Loopback binding by default.** `-addr` defaults to `127.0.0.1:8787` and
  compose publishes to `${ARTEX_BIND:-127.0.0.1}`. Starting without a password on
  a non-loopback address logs a warning.
- **Password changes invalidate sessions.** The token signing key is bound to the
  password hash, so changing the password ends every issued session.

Finish initial setup before you widen the binding.

---

## Network exposure on Linux

Two independent layers, and conflating them is the common mistake:

1. **Which host interface publishes the UI.** `./bind.sh` rewrites `ARTEX_BIND`
   and recreates the container:

   ```bash
   ./bind.sh list     # interfaces with IPv4 addresses on this host
   ./bind.sh local    # 127.0.0.1 only (default)
   ./bind.sh lan      # wired LAN address only
   ./bind.sh wifi     # Wi-Fi address only
   ./bind.sh any      # 0.0.0.0, every interface
   ./bind.sh enp3s0   # a named interface
   ./bind.sh status
   ```

   Publishing to one interface makes the other refuse connections — that is the
   intent. A DHCP address change leaves the pinned address stale, so re-run
   `./bind.sh lan`/`wifi` after the lease moves; `./bind.sh any` survives rotation
   but exposes everything.

2. **The container's own network paths.** `docker-compose.yml` attaches `artex` to
   an `internal` bridge (`internal: true`, no egress) that carries only
   PostgreSQL traffic, and an `external` bridge used as the egress path for scans
   and internet access.

> **Linux advantage over Docker Desktop.** On a Linux host you *can* parent the
> `external` bridge to a physical NIC with `ipvlan`/`macvlan`, giving true
> interface-level isolation. The macOS limitation described in the
> [README](../README.md) (the Docker Desktop VM only sees its own `eth0`/`eth1`)
> does not apply here.

**Firewall caveat.** Docker publishes ports through its own DNAT rules, which
bypass `ufw`'s `INPUT` chain — `ufw allow 8787` will not reliably filter a
compose-published port. Two workable approaches:

- Keep the compose binding on `127.0.0.1` (the default) and reach the UI through
  a reverse proxy or an SSH tunnel. Nothing extra is exposed, so firewall rules
  are unnecessary.
- To expose it on a LAN interface and still filter, restrict at the
  `DOCKER-USER` chain, which Docker evaluates before its own forwarding:

  ```bash
  sudo iptables -I DOCKER-USER -i enp3s0 ! -s 192.168.1.0/24 \
       -p tcp --dport 8787 -j DROP
  ```

  Persist the rule with `iptables-persistent` (or an equivalent), since
  `DOCKER-USER` rules are lost on reboot.

For a bare-metal (non-Docker) deployment, ordinary `ufw`/`nftables` rules on the
listen port work as expected.

---

## Reverse proxy with HTTPS

The frontend and the API/SSE are served by the same backend port, and the live
activity stream connects same-origin by default, so there is no need to set
`NEXT_PUBLIC_SSE_BASE`: expose only `443` publicly and keep `8787` internal.

SSE is a long-lived push connection, so the proxy **must disable buffering**.
Otherwise the browser connects but receives no events, and the activity stream
appears to spin forever.

```nginx
server {
    listen 443 ssl;
    server_name artex.example.com;
    # ssl_certificate     /etc/letsencrypt/live/artex.example.com/fullchain.pem;
    # ssl_certificate_key /etc/letsencrypt/live/artex.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8787;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE: no buffering, long read timeout, HTTP/1.1
        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
    }
}
```

Only set `NEXT_PUBLIC_SSE_BASE` if the event stream must come from a different
origin (for example a separate subdomain) — it is fixed into the static bundle at
`next build` time, so setting it at container runtime has no effect.

---

## Operations

**Health and logs.** `GET /api/health` is unauthenticated and reports the build
version; the UI's `/logs` page streams backend logs. Under Docker use
`docker compose logs -f artex`; under systemd use `journalctl -u artex -f`.

**Backups.** Before any upgrade, back up `./data` (SQLite stores and `jwt.key`),
`./skills`, and the database:

```bash
docker compose exec -T postgres pg_dump -U artex artex > artex-$(date +%F).sql
tar czf artex-data-$(date +%F).tgz data skills
```

**Reset the admin password.** [`reset-password.sh`](../reset-password.sh) writes a
bcrypt hash directly into the database's settings table:

```bash
./reset-password.sh -m docker                  # compose deployment, prompts for the new password
./reset-password.sh --dsn 'postgres://artex:pass@127.0.0.1:5432/artex?sslmode=disable'
```

**Updates.** Schema migrations run automatically on every start, so an update is
a restart. Data (`pgdata`, `./data`, `./skills`) is untouched.

- *UI one-click update* (System settings → version card): downloads the release
  for your platform, verifies it against `SHA256SUMS`, smoke-tests it with `-h`,
  stages `artex.new`, exits `75`, and `start.sh` relaunches into the new build.
  A build that fails to start three times rolls back to `artex.old`. This
  requires `start.sh` as the supervisor, and under Docker it swaps only the
  binary — not the image's toolchain.
- *Script:* [`update.sh`](../update.sh) offers a Docker update (pulling
  `ARTEX_TAG`, recreating the container) or a local recompile.
- *Compose, manual:* `docker compose pull artex && docker compose up -d artex`.
- *Bare metal, manual:* `git pull --ff-only`, rebuild per
  [Build from source](#build-from-source), then `sudo systemctl restart artex`.

Updates restart the process and interrupt running tasks; do them when idle.

---

## Troubleshooting

**`permission denied` connecting to the Docker socket.** Your user is not in the
`docker` group yet. `sudo usermod -aG docker "$USER"`, then log out and back in
(group membership is not applied to the current session).

**`docker compose up` fails on the image pull.** Confirm `ARTEX_TAG` names a
published tag and the registry is reachable; if not, rerun `./install.sh` and
choose the local-compile option.

**Startup exits with "database configuration not found".** Neither
`ARTEX_PG_DSN` nor `config.json` supplied a database. The error names the config
path it inspected; `ARTEX_CONFIG` can point at a different file. Under systemd,
check `EnvironmentFile` is readable by the service group.

**`/setup` rejects the token, or no token appears in the log.** If
`ARTEX_ADMIN_PASSWORD` is set, startup initialises directly and no token is
printed. Otherwise the token is only in the console log — never sent over the
network — so read it from `journalctl`/`docker compose logs`, not the browser.

**UI unreachable on a LAN address.** `./bind.sh status` shows the current binding;
`./bind.sh list` shows candidate interfaces. A stale DHCP lease is the usual
cause — re-run `./bind.sh lan`/`wifi`. Also confirm the firewall is not dropping
the port (see [Network exposure](#network-exposure-on-linux)).

**The activity stream spins forever behind a proxy.** SSE buffering is enabled.
Set `proxy_buffering off` and a long `proxy_read_timeout` as in the Nginx example.

**Port already in use.** Something is listening on `8787` or `8788`. Find it with
`ss -ltnp | grep -E '8787|8788'` and stop it, or move the port with `-addr`/`-proxy`.

**A tool call fails with a raw-socket error.** `nmap`'s SYN scan needs root or
`CAP_NET_RAW`. In Docker the process is root, so this works; on bare metal add
`AmbientCapabilities=CAP_NET_RAW` to the unit or run the service as root.

**`systemctl status artex` shows repeated restarts.** Read
`journalctl -u artex -n 100`. The usual causes are a wrong DSN, PostgreSQL not
yet accepting connections, or a binary that fails its startup smoke test after an
update (the updater rolls back automatically).

**The UPX build will not start.** UPX-packed binaries can conflict with hardened
kernels or security policies; rebuild without `--upx`.

---

## Uninstall

```bash
# Docker deployment
docker compose down            # keeps the pgdata volume, ./data and ./skills
docker compose down -v         # also deletes the pgdata volume — this drops all data
docker image rm ghcr.io/jiwoochris/artex-global:<tag>

# Bare-metal systemd deployment
sudo systemctl disable --now artex
sudo rm /etc/systemd/system/artex.service /etc/artex/artex.env
sudo systemctl daemon-reload
sudo rm -rf /opt/artex         # removes the binary, data/ and skills/
```

---

## Environment reference

`docker-compose.yml` and `.env`:

| Variable | Default | Purpose |
|---|---|---|
| `ARTEX_TAG` | `latest` | Image tag to pull; pin a `v*` release for reproducibility |
| `POSTGRES_PASSWORD` | required | Database password; compose refuses to start without it |
| `ARTEX_BIND` | `127.0.0.1` | Host interface for the published UI port |
| `ARTEX_SETUP_TOKEN` | empty | Pins the first-run setup token instead of logging a random one |
| `ARTEX_ADMIN_PASSWORD` | empty | Sets the admin password at startup, skipping `/setup` (8+ chars) |
| `ARTEX_CORS_ORIGINS` | empty | Comma-separated cross-origin allow-list for `/api` |
| `ARTEX_LANG` | `en` | Server-side default output language (`en`/`ko`/`zh`/`es`/`ja`) |
| `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` | empty | LLM credentials; can also be set in the UI |
| `ARTEX_LLM_PROVIDER`, `ARTEX_LLM_MODEL`, `ARTEX_LLM_BASE_URL`, `ARTEX_LLM_PROXY` | empty | LLM routing, including OpenAI-compatible endpoints |

Binary flags and `config.json`:

| Setting | Default | Purpose |
|---|---|---|
| `-addr` | `127.0.0.1:8787` | HTTP listen address; `:8787` serves other hosts |
| `-proxy` | `127.0.0.1:8788` | Traffic-recording proxy; empty disables it |
| `-data` | `data/` next to the executable | SQLite stores and runtime state |
| `ARTEX_PG_DSN` | — | PostgreSQL DSN; overrides `config.json` |
| `ARTEX_CONFIG` | `./config.json`, then next to the executable | Config file override |
| `ARTEX_SKILL_DIR` | `skills/` next to the executable | Skill directory override |

`config.json` database block:

```json
{
  "database": {
    "host": "127.0.0.1", "port": 5432,
    "user": "artex", "password": "yourpass",
    "dbname": "artex", "sslmode": "disable"
  }
}
```
