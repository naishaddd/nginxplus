#!/bin/sh
# Builds static/vendor/* and static/app.css so the Go binary can embed everything
# (no CDN at runtime -> works offline / air-gapped).
# Usage: ./scripts/build-assets.sh            (needs Node 18+)
#    or: docker run --rm -v "$PWD":/src -w /src node:20-alpine sh scripts/build-assets.sh
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT/web"
if [ -f package-lock.json ] && [ "${NPM_REFRESH:-}" != 1 ]; then npm ci --no-audit --no-fund --silent; else npm install --no-audit --no-fund --silent; fi
NM="$ROOT/web/node_modules"
V="$ROOT/static/vendor"
rm -rf "$V" && mkdir -p "$V/xterm" "$V/fontawesome/css" "$V/fontawesome/webfonts" "$V/fonts"

# Tailwind -> static/app.css
cd "$ROOT"
"$NM/.bin/tailwindcss" -c web/tailwind.config.js -i web/input.css -o static/app.css --minify

# CodeMirror 6 editor + dagre layout → one IIFE bundle (window.NP)
"$NM/.bin/esbuild" web/src/bundle.js --bundle --minify --format=iife --target=es2020 --outfile="$V/bundle.js" --log-level=warning

# xterm
cp "$NM/xterm/css/xterm.css" "$NM/xterm/lib/xterm.js" "$V/xterm/"
cp "$NM/xterm-addon-fit/lib/xterm-addon-fit.js" "$V/xterm/"

# Font Awesome (solid + regular only)
cp "$NM/@fortawesome/fontawesome-free/css/fontawesome.min.css" \
   "$NM/@fortawesome/fontawesome-free/css/solid.min.css" \
   "$NM/@fortawesome/fontawesome-free/css/regular.min.css" "$V/fontawesome/css/"
cp "$NM/@fortawesome/fontawesome-free/webfonts/fa-solid-900.woff2" \
   "$NM/@fortawesome/fontawesome-free/webfonts/fa-regular-400.woff2" "$V/fontawesome/webfonts/"

# Fonts (latin subset only)
CSS="$V/fonts/fonts.css"; : > "$CSS"
for w in 400 500 600 700 800; do
  cp "$NM/@fontsource/inter/files/inter-latin-$w-normal.woff2" "$V/fonts/"
  printf "@font-face{font-family:'Inter';font-weight:%s;font-style:normal;font-display:swap;src:url(inter-latin-%s-normal.woff2) format('woff2')}\n" $w $w >> "$CSS"
done
for w in 400 500 600; do
  cp "$NM/@fontsource/jetbrains-mono/files/jetbrains-mono-latin-$w-normal.woff2" "$V/fonts/"
  printf "@font-face{font-family:'JetBrains Mono';font-weight:%s;font-style:normal;font-display:swap;src:url(jetbrains-mono-latin-%s-normal.woff2) format('woff2')}\n" $w $w >> "$CSS"
done
echo "assets built: $(du -sh "$ROOT/static" | cut -f1) in static/"
