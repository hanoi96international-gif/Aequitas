#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/quantengeld-mindest-pruefer.sh
# mit einem kleinen HTTP-Server, der einen laufenden Knoten spielt.
set -euo pipefail
cd "$(dirname "$0")/.."
WACHE="$PWD/.github/neustart/quantengeld-mindest-pruefer.sh"
REPO=$(sed -n 's/^\tquantenFreischaltungMindestPruefer = \([0-9]\{1,3\}\)$/\1/p' x/humanity/keeper/quanten_geld.go)
[ -n "$REPO" ] || { echo "FEHLER: quantenFreischaltungMindestPruefer nicht lesbar"; exit 1; }
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api"
PORT=$((20000 + RANDOM % 20000))
# Antwortet auf /api/status und /api/quanten/stand mit festen Dateien.
cat > "$D/server.py" <<'PY'
import http.server, sys, os
d = sys.argv[2]
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        name = {"/api/status": "status", "/api/quanten/stand": "stand"}.get(self.path, "")
        f = os.path.join(d, name)
        if not name or not os.path.exists(f):
            self.send_response(404); self.end_headers(); return
        b = open(f, "rb").read()
        self.send_response(200); self.end_headers(); self.wfile.write(b)
    def log_message(self, *a): pass
http.server.HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
python3 -I "$D/server.py" "$PORT" "$D/api" & PID=$!
sleep 1
lauf() { # status-inhalt stand-inhalt erwartung
  printf '%s' "$1" > "$D/api/status"
  if [ -n "$2" ]; then printf '%s' "$2" > "$D/api/stand"; else rm -f "$D/api/stand"; fi
  if bash "$WACHE" "http://127.0.0.1:$PORT" >/dev/null 2>&1; then e=ok; else e=halt; fi
  [ "$e" = "$3" ] || { echo "FEHLER: status=$1 stand=$2 -> $e, erwartet $3"; exit 1; }
  echo "ok: status=$1 stand=$2 -> $e"
}
ANDERS=$((REPO + 1))
lauf "{\"quanten\":{\"mindest_pruefer\":$REPO}}" '' ok                       # gleiche Regel, Stand egal
lauf '{"quanten":{"topf":"0x1"}}' '{"saetze":[]}' ok                         # aelterer Knoten, kein Satz
lauf '{}' '{"saetze":[],"noten":0}' ok                                       # ganz alter Knoten, kein Satz
lauf "{\"quanten\":{\"mindest_pruefer\":$ANDERS}}" '{"saetze":[]}' ok        # andere Zahl, kein Satz
lauf '{"quanten":{"topf":"0x1"}}' '{"saetze":[{"name":"s"}]}' halt          # Satz unter der alten Regel
lauf "{\"quanten\":{\"mindest_pruefer\":$ANDERS}}" '{"saetze":[{"name":"s"}]}' halt
lauf '{"quanten":{"topf":"0x1"}}' '' halt                                    # Stand nicht abrufbar
lauf '{"quanten":{"topf":"0x1"}}' 'kein json' halt                           # Stand kaputt
lauf '{"quanten":{"topf":"0x1"}}' '{"noten":0}' halt                         # Saetze fehlen
lauf '{"quanten":{"topf":"0x1"}}' '{"saetze":"[]"}' halt                     # Saetze kein Array
lauf '{"quanten":{"mindest_pruefer":"x"}}' '{"saetze":[]}' halt              # unlesbar
lauf '{"quanten":{"mindest_pruefer":99999}}' '{"saetze":[]}' halt            # riesige Zahl
lauf '{"quanten":{"mindest_pruefer":"05"}}' '{"saetze":[]}' halt             # fuehrende Null
lauf 'kein json' '{"saetze":[]}' halt                                        # kaputte Antwort
rm -f "$D/api/status"
if bash "$WACHE" "http://127.0.0.1:$PORT" >/dev/null 2>&1; then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
