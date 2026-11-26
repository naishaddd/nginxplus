# AGENTS.md

Instructions for AI coding agents (Claude Code, Codex, Cursor, Copilot, Gemini CLI, Jules, Aider and others) working in this repository. Humans should start with [README.md](README.md) and [CONTRIBUTING.md](CONTRIBUTING.md).

## What this project is

NGINX++ is a sidecar container: a single static Go binary that attaches to an existing NGINX container through the Docker socket and serves a dashboard, a config editor with lint and safe apply/rollback, live traffic analytics, a topology graph with a route tester, and a web terminal. It never modifies the NGINX image itself.

- Language: Go 1.22, standard library plus `gorilla/websocket`. No web framework.
- UI: plain HTML and JavaScript in `static/`, embedded with `go:embed` (`assets.go`). No SPA framework, no CDN.
- Runtime: Docker. The Docker Engine API is called directly over `/var/run/docker.sock` (`internal/docker`).

## Commands

```bash
make test     # go test -race ./...   (no Docker needed; run this after every change)
make vet      # go vet ./...
make lint     # golangci-lint run
make build    # ./bin/nginxpp
make assets   # rebuild static/app.css and static/vendor/ (Node 18+; only if Tailwind classes change)
make demo     # demo NGINX + NGINX++ at http://localhost:8000, setup code "demo"
make demo-down
```

Before finishing, run `gofmt -l .` (it must print nothing), `go vet ./...` and `go test ./...`. CI runs exactly these and also fails if `static/` differs from a fresh `make assets`.

## Layout

| Path | Contents |
|---|---|
| `cmd/nginxpp/` | Entry point |
| `internal/nginx/` | Pure NGINX logic: parse, lint, topology, route matching, diff, snippets. Most changes and tests live here |
| `internal/server/` | HTTP routes, auth, SSE stream, web terminal, config apply workflow |
| `internal/telemetry/` | Access-log streaming, 1m/5m/15m windows, stub_status, upstream probes |
| `internal/docker/` | Docker Engine API client (exec, logs, archive upload) |
| `internal/snapshots/` | Snapshot store on the `/data` volume |
| `internal/auth/`, `internal/config/` | Password/session auth, env-var settings |
| `static/` | UI. `app.css` and `vendor/` are **generated**; do not hand-edit them |
| `web/` | Tailwind and asset-build inputs |
| `examples/demo/` | Demo stack with deliberately broken backends and lint findings |
| `docs/DESIGN.md` | "Signal" design system rules for any UI work |

## Rules (security-critical, do not relax)

1. `internal/nginx` stays pure: no Docker, network or filesystem calls, so it remains unit-testable.
2. Never build shell commands from user input. Use `docker.Exec` with an argument slice and `docker.WriteFile` (Docker archive API) to write files.
3. Validate every config path with `nginx.SafePath`; access is limited to `/etc/nginx/`.
4. Escape every untrusted string rendered in the UI with `escapeHtml` (log lines, config text, hostnames).
5. Any change to NGINX config goes through: snapshot → write → `nginx -t` → reload → restore on failure. Do not add a path that skips this.
6. Every route, including the WebSocket terminal, requires an authenticated session or the API token, and honours `NGINXPP_READONLY`.
7. No new runtime dependencies or CDN assets without a clear reason; the app must work offline.

## Conventions

- Add a lint rule in `internal/nginx/lint.go` and a case in `TestLintRules` (`internal/nginx/nginx_test.go`).
- Recipes and directive help live in `RECIPES` and `NGINX_DOCS` in `static/configflow.js`.
- UI components come from `static/design.css`; follow `docs/DESIGN.md` and check `/styleguide.html`.
- Commits: Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `chore:`). Branches: `feat/<thing>`, `fix/<thing>`.
- Keep PRs focused, include tests, and update `CHANGELOG.md` under `[Unreleased]`.

## Environment variables

`TARGET_CONTAINER`, `PORT`, `NGINXPP_SETUP_CODE`, `NGINXPP_ADMIN_USER`, `NGINXPP_ADMIN_PASSWORD`, `NGINXPP_API_TOKEN`, `NGINXPP_READONLY`, `NGINXPP_DATA_DIR`, `NGINXPP_ACCESS_LOG`, `NGINXPP_MAX_TERMINALS`. See the table in README.md. Never commit real values.
