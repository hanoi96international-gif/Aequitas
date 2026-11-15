#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/offline-nicht-verpasst.sh
# mit einem kleinen HTTP-Server, der einen laufenden Knoten spielt.
set -euo pipefail
cd "$(dirname "$0")/.."
WACHE="$PWD/.github/neustart/offline-nicht-verpasst.sh"
REPO=$(sed -n 's/^\tofflineAbUnix int64 = \([0-9]\{1,\}\)$/\1/p' x/humanity/keeper/offline_bargeld.go)
[ -n "$REPO" ] || { echo "FEHLER: offlineAbUnix nicht lesbar"; exit 1; }
TOPF=$(python3 -c 'import hashlib;print("0x"+hashlib.sha256(b"aequitas-offline-topf-v1").hexdigest()[24:])')
grep -q "TOPF=$TOPF " "$WACHE" || { echo "FEHLER: Topf-Adresse im Waechter stimmt nicht"; exit 1; }
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api"
PORT=$((20000 + RANDOM % 20000))
# Antwortet auf /api/status und /api/balance?wallet=... mit festen Dateien.
cat > "$D/server.py" <<'PY'
import http.server, sys, os
d = sys.argv[2]
class H(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        name = "status" if self.path == "/api/status" else ("balance" if self.path.startswith("/api/balance?wallet=") else "")
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
lauf() { # status-inhalt balance-inhalt jetzt erwartung
  printf '%s' "$1" > "$D/api/status"
  if [ -n "$2" ]; then printf '%s' "$2" > "$D/api/balance"; else rm -f "$D/api/balance"; fi
  if bash "$WACHE" "http://127.0.0.1:$PORT" "$3" >/dev/null 2>&1; then e=ok; else e=halt; fi
  [ "$e" = "$4" ] || { echo "FEHLER: status=$1 balance=$2 jetzt=$3 -> $e, erwartet $4"; exit 1; }
  echo "ok: status=$1 balance=$2 jetzt=$3 -> $e"
}
J=1800000000
if [ "$REPO" = 9223372036854775807 ]; then
  # Schlafend im Repo.
  lauf '{}' '{"balance":0}' $J ok                                   # aelterer Knoten, Topf leer
  lauf '{}' '{"balance":0.000000}' $J ok
  lauf '{}' '{"balance":1.5}' $J halt                               # Topf hat Guthaben
  lauf '{}' '{"balance":"x"}' $J halt                               # unlesbar
  lauf '{}' '' $J halt                                              # Guthaben nicht abrufbar
  lauf '{"offline":{"topf":"0x1"}}' '' $J ok                        # Knoten kennt es schon, schlafend
  lauf '{"offline":{"topf":"0x1","ab":1700000000}}' '' $J halt      # Knoten nimmt es schon an: zurueck auf schlafend waere rueckwirkend
  lauf '{"offline":{"topf":"0x1","ab":"x"}}' '' $J halt             # unlesbar
  lauf '{"offline":{"ab":99999999999999999999}}' '' $J halt         # riesige Zahl
  lauf '{"offline":{"ab":"0001794247200"}}' '' $J halt              # fuehrende Nullen
  lauf 'kein json' '' $J halt                                       # kaputte Antwort
else
  lauf "{\"offline\":{\"topf\":\"0x1\",\"ab\":$REPO}}" '' $((REPO + 999999)) ok   # gleicher Stichtag
  lauf '{"offline":{"topf":"0x1"}}' '' $((REPO - 7200)) ok                      # schlafend, zwei Stunden voraus
  lauf '{"offline":{"topf":"0x1"}}' '' $((REPO - 600)) halt                     # nur zehn Minuten voraus
  lauf '{"offline":{"topf":"0x1"}}' '' $((REPO + 60)) halt                      # schon vorbei
  lauf '{"offline":{"topf":"0x1","ab":1700000000}}' '' $((REPO - 7200)) halt    # nimmt schon an
  lauf 'kein json' '' $((REPO - 7200)) halt
fi
rm -f "$D/api/status"
if bash "$WACHE" "http://127.0.0.1:$PORT" 0 >/dev/null 2>&1; then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
