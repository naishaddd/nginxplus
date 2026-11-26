# NGINX++

**A dashboard, config editor and web terminal for any NGINX container. Attach it, don't modify NGINX.**

[![CI](https://github.com/naishaddd/nginxplus/actions/workflows/ci.yml/badge.svg)](https://github.com/naishaddd/nginxplus/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go 1.22](https://img.shields.io/badge/Go-1.22-00ADD8.svg)](go.mod)
[![Release](https://img.shields.io/github/v/release/naishaddd/nginxplus?include_prereleases)](https://github.com/naishaddd/nginxplus/releases)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

NGINX++ runs as a small sidecar container (a ~6 MB static Go binary, ~27 MB image). It connects to your existing NGINX container through the Docker socket and gives you:

- **Safe config editing.** Every change shows a diff, lint findings, and a dry-run `nginx -t` before you apply. Apply takes an undo snapshot first, reloads NGINX, and restores the old file automatically if the reload fails.
- **A linter for real NGINX pitfalls.** It catches `proxy_pass` trailing-slash mistakes, `Host` headers that aren't inherited, `add_header` inheritance, `if` inside `location`, weak TLS versions, `server_tokens` and `autoindex`. Findings appear live as you type, with links to the docs.
- **Snapshots and rollback.** Snapshots are stored on NGINX++'s own volume, so they survive NGINX container re-creation. Each has a message and a real diff, and you can export them all as `.tar.gz`.
- **Recipe builder.** Forms for rate limiting, HTTPS redirect, CORS, caching, WebSocket proxy, reverse proxy and basic auth. Each is validated in a sandbox and can be inserted at the editor cursor.
- **Live traffic analytics.** The access log is streamed (each line counted once) into real 1m / 5m / 15m windows. You get requests per second, status codes, top endpoints and top client IPs.
- **CodeMirror editor** with NGINX syntax highlighting, inline lint markers, hover docs and directive autocomplete.
- **A live topology graph built from your real config** (ports → servers → locations → upstreams → backends) with zoom, a minimap, click-to-source and live backend health. It includes a **route tester** that answers "which block handles `https://shop.example.com/api/users`?", using NGINX's own matching rules and explaining each step.
- **Upstream health, SSL expiry and security grade.**
- **A web terminal** inside the NGINX container, plus a split view with live logs.
- **Works offline.** All UI assets are embedded in the binary. Nothing is loaded from a CDN.

<!-- TODO: add a screenshot or GIF here: docs/screenshot.png -->

---

## Quick start

### Try the demo (30 seconds)

```bash
git clone https://github.com/naishaddd/nginxplus.git && cd nginxplus
make demo
open "http://localhost:8000"     # first run: create the admin account with the setup code make printed
```

This starts a demo NGINX with 9 virtual hosts, 8 upstreams and about 30 locations (a service map of about 60 nodes), with some deliberately dead backends so you can see degraded and down states, plus a few lint findings. NGINX++ is attached to it. Run `make demo-down` to stop it.

### Attach to your own NGINX container

```bash
TARGET_CONTAINER=my-nginx docker compose up -d
docker logs nginxpp | grep "Setup code"
open "http://localhost:8000"
```

**First run:** NGINX++ asks you to create the admin account (username and password, at least 10 characters). To prove you're the operator, you also enter the one-time **setup code** printed in the container logs. After that you sign in normally. Sessions last 7 days, and changing the password (Settings → Account) signs out every other session.

Forgot the password? Delete `auth.json` from the `/data` volume and restart, and you'll go through setup again. Snapshots are kept.

---

## Configuration

| Variable | Default | Purpose |
|---|---|---|
| `TARGET_CONTAINER` | `my-nginx` | Name or ID of the NGINX container to attach to |
| `PORT` | `8000` | Dashboard port |
| `NGINXPP_SETUP_CODE` | *(generated)* | One-time code required to create the admin account on first run. It's printed in the logs if not set |
| `NGINXPP_ADMIN_USER` / `NGINXPP_ADMIN_PASSWORD` | | Create the admin account at startup with no setup screen, for unattended deploys. Remove the password from your config afterwards |
| `NGINXPP_API_TOKEN` | | Optional `Authorization: Bearer` token for scripts and CI |
| `NGINXPP_READONLY` | `false` | `true` disables saving, reload, rollback and the web terminal |
| `NGINXPP_DATA_DIR` | `/data` | Where snapshots are stored. Mount a volume here (the compose file does) |
| `NGINXPP_ACCESS_LOG` | `/var/log/nginx/access.log` | Access log to stream. If it links to `/dev/stdout` (official image), Docker logs are followed instead |
| `NGINXPP_MAX_TERMINALS` | `5` | Maximum concurrent web terminal sessions |

Analytics expect NGINX's default `combined` log format.

---

## How it works

```
 browser ──HTTPS/SSH tunnel──▶  NGINX++ (Go, :8000)
                                 ├─ REST API + SSE metrics stream
                                 ├─ WebSocket terminal
                                 └─ embedded UI (static/)
                                        │  /var/run/docker.sock
                                        ▼
                               Docker Engine API
                                 ├─ exec: nginx -T / -t / -s reload, cat
                                 ├─ archive PUT: write config files (no shell)
                                 └─ logs (follow): access log stream
                                        │
                                        ▼
                                your NGINX container (unchanged)
```

Saving a config follows these steps: **preview** (diff, lint, dry-run `nginx -t`) → **snapshot** the current file → **write** → **`nginx -t`** → **reload** → if the reload fails, **restore and reload again**.

---

## Security model

NGINX++ uses the Docker socket, which is **root-equivalent on the host**, and it offers a shell into the NGINX container. Treat access to it like SSH access.

- Every page, API route and the terminal require a signed-in admin session (bcrypt-hashed password, signed HttpOnly SameSite=Strict cookie). Failed logins are rate-limited (5 per 15 minutes per IP), and cross-origin write requests are rejected. The provided compose file binds to `127.0.0.1` only. To reach NGINX++ remotely, use an SSH tunnel, a VPN or a TLS reverse proxy.
- The terminal rejects cross-origin WebSocket connections. Files can only be accessed under `/etc/nginx/`, and paths are cleaned before checking. Files are written through the Docker archive API, never a shell. Untrusted values such as log lines are HTML-escaped in the UI.
- Use `NGINXPP_READONLY=true` when you only need observability.

See [SECURITY.md](SECURITY.md) for how to report vulnerabilities and for a hardening checklist.

---

## Development

```bash
make test     # unit tests. No Docker needed
make demo     # full stack with a demo NGINX
make build    # ./bin/nginxpp
make assets   # rebuild Tailwind CSS + vendored fonts/icons (only if you change classes)
```

```
cmd/nginxpp/        entry point
internal/nginx/     pure NGINX logic: parse, topology, lint, diff, snippets (good first contributions)
internal/telemetry/ log streaming, time windows, stub_status, upstream probes
internal/server/    HTTP routes, auth, SSE, terminal, config workflow
internal/docker/    Docker Engine API client
internal/snapshots/ snapshot store
static/             UI (plain HTML + JS), embedded via go:embed
```

UI work follows the **Signal design system**: see [docs/DESIGN.md](docs/DESIGN.md) and `/styleguide.html` in a running instance.

See [CONTRIBUTING.md](CONTRIBUTING.md) for step-by-step guides to adding a lint rule, a recipe or directive help. See the [roadmap](ROADMAP.md) for what's planned.

## License

[MIT](LICENSE)
