# NGINX++ — Improvement Plan

Working checklist for turning the project (formerly "Mora") into an open-source-ready flagship.
Items are done in order; tick them as they land.

## 0. Security (blockers before public release)
- [x] Token authentication for all API routes, the web terminal and the UI (`NGINXPP_TOKEN`, auto-generated if unset)
- [x] WebSocket Origin check (prevent cross-site WebSocket hijacking of the shell)
- [x] Replace `sh -c "cat << EOF"` file writes with the Docker archive API (no shell injection)
- [x] Path traversal: `path.Clean` + prefix check on every file path
- [x] Snapshot IDs validated; rollback restores the snapshot's own file (not a hardcoded path)
- [x] Validate-before-save: write → `nginx -t` → auto-restore previous content on failure
- [x] Publish dashboard on `127.0.0.1` only in docker-compose by default
- [x] Read-only mode (`NGINXPP_READONLY=true`) disables writes, reload, rollback and terminal

## 1. Honesty & branding
- [x] Rename Mora → NGINX++ (README, UI, logs, module, compose)
- [x] Remove third-party "New Relic" branding → "NGINX++ Design Tokens"
- [x] Rename "AI Copilot" to "Snippet Templates" until a real pluggable LLM provider exists
- [x] Snapshot button uses the currently open file, not a hardcoded path
- [x] Measure and publish real binary/image size (6.0 MB binary, 25.4 MB image; runtime memory still to measure)
- [x] Fix placeholder repo URLs and badges (now naishaddd/nginxplus)

## 2. UI & canvas
- [x] Settings wired (static/settings.js): snippets tab visibility, undo-snapshot on apply (server honours `snapshot:false`), Upstreams auto-refresh interval. Fixed: `saveSetting` was undefined
- [x] Topology rebuilt from REAL config (the old view was a hardcoded fake diagram): dagre auto-layout SVG, wheel zoom, drag pan, fit, clickable minimap
- [x] Parser rewritten on the directive tree: file + line for every server/location/upstream; click node → editor at that line; hover/focus traces upstream+downstream path
- [x] Backend nodes/edges coloured from live health probes (latency on edges not yet)
- [x] Filter box, collapse-locations toggle, route tester (`/api/nginx/route`: nginx server_name + location algorithm, step-by-step explanation, highlights the path)
- [x] CodeMirror 6 (bundled, offline): nginx syntax, line numbers, lint gutter + inline markers, hover docs, directive autocomplete, search, ⌘S → review
- [ ] Map `nginx -t` error output (file:line) to inline editor markers
- [ ] Command palette (⌘K), resizable split panes, ⌘S = save + test
- [ ] Toasts instead of `alert()`, skeleton loaders, persistent "config invalid" banner
- [~] Accessibility: graph nodes keyboard-focusable (Enter opens), aria labels, text badges (up/down) besides colour, `prefers-reduced-motion`. Full audit still to do

## 3. Performance
- [x] Remove runtime CDNs: Tailwind compiled at build time (scripts/build-assets.sh), fonts/icons/xterm vendored under static/ (852 KB total), embedded via go:embed
- [x] Streaming log ingestion: Docker logs follow (stdout) or `tail -F` exec stream; each line counted once (telemetry.go)
- [x] Real 1m/5m/15m windows via 900-slot per-second ring buffer; top endpoints/IPs now actually sorted
- [x] SSE push channel `/api/stream` drives the live Analytics tab (QPS + 60s sparkline + window picker)
- [x] Cache `nginx -T` parse (15s TTL); invalidated on save/reload/rollback
- [x] Concurrent upstream probes (32 in flight, 1s timeout); removed fake "healthy" for 10.0.* addresses
- [x] Cap concurrent terminal sessions (`NGINXPP_MAX_TERMINALS`, default 5)
- [x] Fix stored XSS: access-log paths/IPs and other untrusted fields were injected via innerHTML unescaped
- [ ] Probes dial from the NGINX++ container, so upstream hostnames/IPs only resolve if it shares nginx's network; probe from inside the nginx container or document `network_mode: container:my-nginx`
- [ ] stub_status is hardcoded to `curl https://127.0.0.1:8443/metrics` (curl is missing from some images, e.g. the Debian-based `nginx`); make the URL configurable and fetch without curl
- [ ] Live/browser test: blocked (permissions earlier; host disk 99% full prevented pulling a headless browser image)

## 4. Developer config-editing experience
- [x] Full flow: Review & Apply (⌘S) → diff + lint + dry-run `nginx -t` → message → apply & reload → auto-restore if reload fails (configflow.go/.js)
- [ ] Auto-rollback when upstream health or 5xx rate degrades after a reload (not only on reload failure)
- [x] Linter (lint.go): weak TLS, server_tokens, autoindex, proxy_pass slash mapping, missing/uninherited Host header, add_header inheritance, `if` in location; live Problems panel with click-to-line
- [x] Directive reference bar for the directive under the cursor (~40 directives, links to nginx.org); true hover tooltips come with CodeMirror (section 2)
- [x] Recipe builder forms (rate limit, HTTPS redirect, CORS, cache, WebSocket, reverse proxy, basic auth) → validate in sandbox → copy / insert at cursor
- [x] Sandbox fix: http-level directives (limit_req_zone, map, upstream…) are placed outside the test server block; sandbox listens on 127.0.0.1 only
- [x] Persistent snapshots on the NGINX++ data volume (`/data`), with messages; export as .tar.gz
- [ ] Snapshot import
- [x] Real snapshot diff (LCS line diff with collapsed context); commit message stored
- [ ] Support bare-metal nginx and multiple target containers

## 5. Open-source structure
- [x] Split into `cmd/nginxpp` + `internal/{config,docker,nginx,telemetry,snapshots,server}`; UI embedded from root package `assets.go`
- [x] Unit tests: docker stream demux, traffic windows, log parsing, lint rules, diff, snippet split, path guard
- [ ] Tests for `nginx.ParseDump` / `BuildTopology` with fixture configs, and `internal/server` handler tests with a fake Docker
- [x] LICENSE (MIT), CONTRIBUTING (with "common tasks" table), SECURITY (private reporting + hardening), CODE_OF_CONDUCT, issue forms, PR template
- [x] CI: gofmt, vet, test -race, golangci-lint (clean), asset build up-to-date check, docker build; release: goreleaser binaries + multi-arch GHCR image on tags; Dependabot; Makefile
- [x] README rewritten: quickstart with `make demo` (examples/demo), env table, architecture diagram, security model
- [ ] Add a real screenshot/GIF to the README (placeholder TODO left)
- [x] ROADMAP rewritten as Shipped / Now / Next / Later with 🟢 good-first-issue markers
- [ ] Create the GitHub labels/issues from the roadmap once the repo is public

## 6. Design system & accounts (added 2026-11-25)
- [x] "Signal" design system: tokens/components in `static/design.css`, rules in `docs/DESIGN.md`, live reference at `/styleguide.html`; own logo (`static/logo.svg`); light "paper" default with always-dark console surfaces
- [x] Single sidebar nav + top bar (breadcrumb, target status, user menu); Overview rebuilt with live KPIs (old tiles showed fake "5 / 5", "Grade A+")
- [x] Admin account: first-run setup (setup code from logs, or NGINXPP_ADMIN_USER/PASSWORD), bcrypt, signed session cookies, login rate limit, change password, sign out; optional NGINXPP_API_TOKEN for automation (`internal/auth`, `internal/server/authn.go`)
- [ ] Restyle remaining tab bodies (Upstreams, SSL, Snapshots, Traffic) fully onto `np-*` components and remove Tailwind colour utilities
