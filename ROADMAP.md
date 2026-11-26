# Roadmap

What's planned for NGINX++, grouped by priority. Items marked 🟢 are good first issues. The detailed engineering checklist is in [docs/IMPROVEMENT_PLAN.md](docs/IMPROVEMENT_PLAN.md).

## ✅ Shipped

- Token auth, read-only mode, cross-origin WebSocket protection, path validation, shell-free file writes
- Config workflow: preview (diff, lint, dry-run `nginx -t`) → apply & reload → automatic restore if the reload fails
- Linter for common NGINX pitfalls; directive reference in the editor
- Persistent snapshots with messages, real diffs and `.tar.gz` export
- Recipe builder (rate limit, HTTPS redirect, CORS, cache, WebSocket, reverse proxy, basic auth) with sandbox validation
- Streaming traffic analytics with real 1m / 5m / 15m windows over SSE
- Concurrent upstream health probes, SSL expiry and security grade, topology view
- Web terminal and split view with live logs
- CodeMirror 6 editor (syntax, lint markers, hover docs, autocomplete)
- Real topology graph (auto-layout, zoom, minimap, click-to-source, live health) and a route tester
- Working settings (snippets tab, undo snapshots, probe interval)
- Offline UI (no CDNs), single static binary, CI, multi-arch images on GHCR

## 🔜 Now

- **Upstream probes** from inside the NGINX network namespace, so hostnames resolve correctly
- **Configurable `stub_status` URL**, fetched without depending on `curl`
- **Show `nginx -t` errors inline** in the editor, at the right file and line
- Latency labels on topology edges; evaluate nested locations in the route tester
- Accessibility audit (keyboard navigation, screen-reader pass)

## ⏭️ Next

- Automatic rollback when the 5xx rate or upstream health gets worse after a reload
- Snapshot import 🟢
- More lint rules 🟢: `ssl_ciphers` weaknesses, missing `client_max_body_size` on upload routes, `alias` without a trailing slash, duplicate `listen ... default_server`
- More recipes 🟢: gzip, security headers, SPA fallback, proxy cache
- Replace the remaining `alert()` popups with toasts 🟢; full keyboard navigation and accessibility pass
- A command palette (⌘K)

## 🌅 Later

- Bare-metal NGINX (no Docker) and multiple target containers
- An LLM-backed rule generator (pluggable provider) whose output always goes through the sandbox `nginx -t` and the linter
- Kubernetes support (ingress-nginx and NGINX pods)
- Prometheus `/metrics` endpoint for NGINX++'s own traffic aggregates
