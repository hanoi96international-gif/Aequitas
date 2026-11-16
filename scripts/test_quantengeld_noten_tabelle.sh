#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/quantengeld-noten-tabelle.sh
# mit einem kleinen HTTP-Server, der einen laufenden Knoten spielt.
set -euo pipefail
cd "$(dirname "$0")/.."
WACHE="$PWD/.github/neustart/quantengeld-noten-tabelle.sh"
grep -q '"noten_tabelle": true' x/humanity/keeper/quanten_geld.go || { echo "FEHLER: der Knoten meldet noten_tabelle nicht"; exit 1; }
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api"
PORT=$((20000 + RANDOM % 20000))
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
lauf '{"quanten":{"noten_tabelle":true}}' '' ok                     # schon neues Format
lauf '{"quanten":{"topf":"0x1"}}' '{"noten":0,"saetze":[]}' ok        # alt, keine Note
lauf '{}' '{"noten":0}' ok                                            # ganz alt, keine Note
lauf '{"quanten":{"topf":"0x1"}}' '{"noten":3}' halt                  # alt, mit Noten
lauf '{"quanten":{"noten_tabelle":"ja"}}' '{"noten":0}' halt          # unlesbar
lauf '{"quanten":{"topf":"0x1"}}' '' halt                             # Stand nicht abrufbar
lauf '{"quanten":{"topf":"0x1"}}' 'kein json' halt                    # Stand kaputt
lauf '{"quanten":{"topf":"0x1"}}' '{"noten":"0"}' halt                # Zahl als Text
lauf '{"quanten":{"topf":"0x1"}}' '{"noten":-1}' halt                 # negativ
lauf '{"quanten":{"topf":"0x1"}}' '{"saetze":[]}' halt                # Feld fehlt
lauf 'kein json' '{"noten":0}' halt                                   # kaputte Antwort
rm -f "$D/api/status"
if bash "$WACHE" "http://127.0.0.1:$PORT" >/dev/null 2>&1; then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
