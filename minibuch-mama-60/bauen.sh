#!/usr/bin/env bash
# Baut aus inhalt/*.md das fertige PDF:  bau/minibuch.pdf
#
#   ./bauen.sh
#
set -euo pipefail
cd "$(dirname "$0")"

# --- Node finden ------------------------------------------------------------
NODE_BIN="$(command -v node || true)"
[ -z "$NODE_BIN" ] && [ -x /opt/node22/bin/node ] && NODE_BIN=/opt/node22/bin/node
if [ -z "$NODE_BIN" ]; then
  echo "Node.js nicht gefunden. Installieren: https://nodejs.org" >&2
  exit 1
fi

# --- HTML bauen -------------------------------------------------------------
"$NODE_BIN" werkzeug/bauen.mjs

# --- Browser finden ---------------------------------------------------------
BROWSER=""
for kandidat in \
  /opt/pw-browsers/chromium \
  "$(command -v chromium || true)" \
  "$(command -v chromium-browser || true)" \
  "$(command -v google-chrome || true)" \
  "$(command -v google-chrome-stable || true)" \
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  "/Applications/Chromium.app/Contents/MacOS/Chromium"
do
  if [ -n "$kandidat" ] && [ -x "$kandidat" ]; then BROWSER="$kandidat"; break; fi
done

if [ -z "$BROWSER" ]; then
  echo ""
  echo "  Kein Chrome/Chromium gefunden — das HTML liegt aber fertig unter:"
  echo "     bau/minibuch.html"
  echo "  Öffne es im Browser und drucke es mit Strg+P / Cmd+P als PDF"
  echo "  (Papierformat A5, Ränder: keine, Hintergrundgrafiken: an)."
  exit 0
fi

# --- PDF drucken ------------------------------------------------------------
"$BROWSER" \
  --headless \
  --disable-gpu \
  --no-sandbox \
  --no-pdf-header-footer \
  --print-to-pdf-no-header \
  --print-to-pdf="$PWD/bau/minibuch.pdf" \
  "file://$PWD/bau/minibuch.html" 2>/dev/null

echo "  Fertig:  bau/minibuch.pdf"
echo ""
