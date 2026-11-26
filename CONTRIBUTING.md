# Contributing to NGINX++

Thanks for helping! This guide gets you from clone to merged PR.

## Quick start

```bash
git clone https://github.com/naishaddd/nginxplus.git && cd nginxplus
make test        # unit tests. No Docker needed
make demo        # demo nginx + NGINX++ → prints the setup code, then open http://localhost:8000
make demo-down
```

Requirements: Go 1.22+, Docker (for the demo), Node 18+ only if you change CSS classes.

## Project layout

```
cmd/nginxpp/          entry point (main.go)
internal/config/      env var settings
internal/docker/      Docker Engine API client (exec, streams, file upload)
internal/nginx/       pure nginx logic: parse, topology, lint, diff, snippets  ← easiest place to start
internal/telemetry/   access-log streaming, 1m/5m/15m windows, stub_status, upstream probes
internal/snapshots/   snapshot store on the data volume
internal/server/      HTTP routes, auth, SSE stream, web terminal, config workflow
static/               the UI (plain HTML + JS), embedded into the binary
  index.html          main page
  configflow.js       editor / snapshots / recipes logic
  app.css, vendor/    BUILD OUTPUT of scripts/build-assets.sh (committed)
web/                  Tailwind config + asset build inputs
examples/demo/        self-contained demo stack
docs/                 plans and design notes
```

## Common tasks

| I want to… | Edit | Test with |
|---|---|---|
| Add a **lint rule** | `internal/nginx/lint.go` | add a case to `TestLintRules` in `internal/nginx/nginx_test.go` |
| Add a **recipe** form | `RECIPES` in `static/configflow.js` | Snippets tab → Validate |
| Add **directive help** | `NGINX_DOCS` in `static/configflow.js` | Editor tab, cursor on directive |
| Change **UI styles** | Tailwind classes in `static/*` | `make assets`, reload |
| Add an **API route** | `internal/server/` | `internal/server/authn_test.go`-style httptest, or `make demo` |
| Build or restyle **UI** | `static/design.css` components (see `docs/DESIGN.md`) | `/styleguide.html` |

## Rules of the road

- **Keep `internal/nginx` pure.** No Docker or network calls there, so it stays unit-testable.
- **Never build shell commands from user input.** Use `docker.Exec` with an argument slice, and `docker.WriteFile` for writing files.
- **Validate every path** with `nginx.SafePath` and escape every untrusted string in the UI with `escapeHtml`.
- **Anything that changes config** must go through `nginx -t` and keep an undo snapshot.
- Run `gofmt`, `go vet ./...`, and `go test ./...` before pushing. CI runs the same checks.
- If you change Tailwind classes, run `make assets` and commit `static/app.css`.

## Pull requests

1. Fork the repo and create a branch: `feat/<thing>` or `fix/<thing>`.
2. Keep PRs focused, and add or adjust tests.
3. Fill in the PR template. Include a screenshot for UI changes.
4. Use clear commit messages. Conventional Commits (`feat:`, `fix:`, `docs:`) are welcome.

Look for issues labelled **good first issue** to get started.
