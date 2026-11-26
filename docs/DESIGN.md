# NGINX++ Design System: "Signal"

This guide covers how every NGINX++ screen should look and behave. Tokens and components live in [`static/design.css`](../static/design.css). To see every component rendered live, sign in and open **`/styleguide.html`**.

> **Inspiration, not imitation.** The look follows the style of modern observability products: warm off-white pages, near-black "console" surfaces, pill-shaped buttons, a single bright accent and Inter type. The colours, logo and name are NGINX++'s own. **Never copy another company's brand assets or exact brand colours.**

---

## 1. Principles

1. **Data first, chrome second.** Every pixel of decoration has to earn its place. Prefer numbers, states and source lines over ornament.
2. **Honest UI.** Never show a placeholder value as if it were real. If there's no data, show `—` and say why.
3. **One accent.** Signal Green is for primary actions, "healthy", focus and the current selection. Nothing else uses it.
4. **Colour carries meaning only through status tokens** (ok, warn, error, info). Always pair colour with text or an icon.
5. **Two surfaces.** *Paper* (light, warm) for reading and forms. *Console* (always dark) for the editor, terminal, service map and diffs. The **sidebar follows the theme**: a light rail in light mode, console in dark mode (`--np-nav-*` tokens).
6. **Safe by default.** Destructive actions are never the primary button. Anything that changes NGINX goes through review (diff → lint → `nginx -t`).

---

## 2. Tokens

Use the CSS variables, never raw hex values, in new code. Every name below starts with `--np-`, which is left off in the tables.

### Colour

| Token | Light | Dark | Use |
|---|---|---|---|
| `accent` | `#10B981` | `#34D399` | Primary buttons, active nav item, focus ring, healthy |
| `accent-hover` | `#0EA371` | `#4ADE9F` | Hover on primary |
| `accent-ink` | `#04140B` | same | Text on accent |
| `paper` | `#EFEEE8` | `#0B1215` | Page background (warm, never pure white) |
| `surface` | `#FFFFFF` | `#121A1E` | Cards |
| `surface-2` | `#F7F6F1` | `#1A2328` | Inputs, nested panels, table headers |
| `ink` | `#0B1215` | `#EEF0EA` | Primary text |
| `ink-muted` | 62% ink | `#9AA3A7` | Secondary text, labels |
| `ink-faint` | 42% ink | `#6B757A` | Hints, metadata |
| `line` / `line-strong` | 10% / 18% ink | 9% / 16% | Borders, dividers |
| `console`, `console-2` | `#0A1013`, `#121A1E` | same | Always-dark surfaces |
| `ok` / `warn` / `error` / `info` | `#16A34A` / `#D97706` / `#DC2626` / `#2563EB` | brighter variants | **Status only** |
| `*-soft` | 12–14% tint | | Badge and alert backgrounds |

### Type

- **Inter** for UI and **JetBrains Mono** for code, paths, IPs, IDs and numbers in tables.
- Scale: `xs 11` · `sm 12.5` · `base 14` · `lg 16` · `xl 20` · `2xl 28` (px).
- Headings use weight 600–700 and `letter-spacing: -0.02em`. Eyebrows and labels are 11px uppercase with `0.08em` tracking.
- Numbers in KPIs and tables use `font-variant-numeric: tabular-nums`.

### Shape and space

- **4px grid:** `4 · 8 · 12 · 16 · 24 · 32`.
- **Radii:** `sm 6` (tags) · `md 10` (inputs, alerts) · `lg 16` (cards) · `xl 24` (hero panels) · `pill` (buttons, badges, nav items).
- **Shadows:** `shadow-sm` on cards in light mode only. `shadow-md` on menus and dialogs.

---

## 3. Components

| Component | Class | Rules |
|---|---|---|
| Button | `np-btn` + `--primary` / `--secondary` / `--ghost` / `--dark` / `--danger`, sizes `--sm` / `--lg`, `--icon`, `--block` | Always pill-shaped. **At most one `--primary` per view.** Icon-only buttons need `aria-label`. |
| Card | `np-card` (+ `--pad`, `--console`) | One idea per card. Header = `np-card-header` with an eyebrow and a title on the left, actions on the right. |
| KPI tile | `np-kpi` with `__label`, `__value`, `__meta` | Live values only. Use `—` when there's no data. The meta line explains the unit or window. |
| Badge | `np-badge` + `--ok` / `--warn` / `--error` / `--info` | Status = dot (`np-dot`) or icon **plus** a word. Never colour alone. |
| Tag | `np-tag` | Uppercase category labels ("First run", "Open source"). Not for status. |
| Input | `np-label` + `np-input` / `np-select` + `np-help` | Every input has a visible label. Errors: `aria-invalid="true"` plus a message in `np-alert--error`. |
| Alert | `np-alert` + status | For inline results of an action. Use toasts only for transient confirmations. |
| Table | `np-table` | Mono font for IPs, paths and numbers. Hovered rows use `surface-2`. |
| Console pane | `np-console` | For code, logs and command output. Always dark, in both themes. |

---

## 4. Layout

- **App shell:** themed **icon rail** (64px, icons only; the label appears as a pill on hover or keyboard focus, from `data-title`). The top bar shows the active section's pages as a **pill nav** (`np-pillnav`); on the right are the target status and user menu. Content sits on `paper` with 24px padding.
- **Navigation groups:** *Monitor* (read-only views), *Configure* (changes NGINX), *Tools* (Terminal, with a Single/Split layout).
- **Full-bleed tools:** the service map and the config editor give the workspace priority: one slim toolbar row, with secondary tools (route tester, node details) in floating panels over the canvas.
- **Editor workspace:** explorer tree on the left, tabs on top (a dot marks unsaved changes), console editor below, and Problems at the bottom. New pages go into one of these groups.
- **Grids:** KPI rows are 2 columns on small screens, 4–5 on `xl`. Use a gap of 16px between cards and 24px between sections.
- **Auth pages:** a split screen with the console brand panel on the left and the form on the right. The brand panel is hidden below 860px.

---

## 5. Writing

- Use **sentence case** everywhere ("Create snapshot", not "Create Snapshot").
- Buttons say what happens: "Apply & reload", "Restore & reload", "Sign out". Avoid "OK" and "Submit".
- Errors say what went wrong **and** what to do: "Wrong setup code. Check the container logs (`docker logs nginxpp`)."
- Don't use marketing words in the product UI ("ultra", "agentic", "AI-powered") unless the feature literally is that.

---

## 6. Accessibility checklist (required for every PR touching UI)

- [ ] Text contrast is ≥ 4.5:1 (≥ 3:1 for text 18px and larger). The muted tokens meet this on `paper` and `surface`.
- [ ] Everything works with the keyboard, and the focus ring is visible (`:focus-visible` uses the accent).
- [ ] Status is never conveyed by colour alone.
- [ ] Icon-only controls have `aria-label`. Live regions (`aria-live="polite"`) are used for async results.
- [ ] Motion respects `prefers-reduced-motion` (already handled globally in `design.css`).

---

## 7. Do and don't

| ✅ Do | ❌ Don't |
|---|---|
| Use `var(--np-accent)` | Hard-code `#10B981` or Tailwind `emerald-*` in new markup |
| Put the main action on the right of a card header | Put two green buttons side by side |
| Use `—` plus an explanation when data is missing | Ship sample numbers ("5 / 5", "Grade A+") as defaults |
| Use console surfaces for code, logs and the map | Use dark cards for ordinary forms in light mode |
| Add new components to `design.css` and `/styleguide.html` | Write one-off inline `<style>` blocks |
