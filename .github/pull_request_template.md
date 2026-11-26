## What & why

<!-- What does this change and why? Link the issue: Fixes #123 -->

## How tested

- [ ] `go test ./...` passes
- [ ] Tried it with `make demo` (for UI or API changes)
- [ ] Ran `make assets` and committed `static/app.css` (if Tailwind classes changed)

## Screenshots (UI changes)

## Checklist

- [ ] No shell commands built from user input; paths validated with `nginx.SafePath`
- [ ] Untrusted strings escaped in the UI (`escapeHtml`)
- [ ] Docs or README updated if behaviour or env vars changed
