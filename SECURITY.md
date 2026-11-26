# Security Policy

NGINX++ talks to the Docker socket, which is **root-equivalent on the host**, and exposes a web shell into the nginx container. We take reports seriously.

## Reporting a vulnerability

Please **do not open a public issue**. Use GitHub's private vulnerability reporting instead: open the **Security** tab, then click **Report a vulnerability**.

Include affected version or commit, reproduction steps, and impact. We aim to acknowledge within 3 business days and to ship a fix or mitigation within 30 days for high-severity issues.

## Supported versions

Only the latest release receives security fixes.

## Hardening checklist for operators

- Complete first-run setup immediately after deploying. Until then, anyone with the setup code from the logs can claim the admin account. Use a strong password, and never expose NGINX++ to the internet without TLS in front of it.
- Bind to localhost (the default in the provided compose file) and reach it over an SSH tunnel or VPN.
- Use `NGINXPP_READONLY=true` where you only need observability.
- Consider a Docker socket proxy that allows only `exec`, `logs`, `archive`, and `inspect` for the target container.

## Security model (summary)

- Every route requires an admin session: bcrypt password, HMAC-signed HttpOnly SameSite=Strict cookie, 7-day expiry, invalidated when the password changes. Logins are rate-limited per IP. An optional `NGINXPP_API_TOKEN` allows Bearer access for automation.
- The WebSocket terminal rejects cross-origin connections.
- File access is restricted to `/etc/nginx/`, and paths are cleaned before checking.
- Files are written through the Docker archive API, never through a shell.
- Untrusted data (log lines, paths, IPs, certificate fields) is HTML-escaped in the UI.
