#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/b1b-stichtag-nicht-verpasst.sh
# mit einem kleinen HTTP-Server, der einen laufenden Knoten spielt.
set -euo pipefail
cd "$(dirname "$0")/.."
REPO=$(sed -n 's/^const bescheinigungRegisterAbUnix int64 = \([0-9]\{1,\}\)$/\1/p' x/humanity/keeper/bio_bescheinigung_register.go)
[ -n "$REPO" ] || { echo "Stichtag schlafend -- nichts zu testen"; exit 0; }
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api"
PORT=$((20000 + RANDOM % 20000))
(cd "$D" && python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) & PID=$!
sleep 1
lauf() { # status-inhalt jetzt erwartung
  printf '%s' "$1" > "$D/api/status"
  if bash .github/neustart/b1b-stichtag-nicht-verpasst.sh "http://127.0.0.1:$PORT" "$2" >/dev/null 2>&1; then e=ok; else e=halt; fi
  [ "$e" = "$3" ] || { echo "FEHLER: status=$1 jetzt=$2 -> $e, erwartet $3"; exit 1; }
  echo "ok: status=$1 jetzt=$2 -> $e"
}
lauf "{\"bescheinigung_konsens\":{\"register_ab\":$REPO}}" $((REPO + 999999)) ok   # gleicher Stichtag: immer erlaubt
lauf '{"bescheinigung_konsens":{"ab_unix":1}}' $((REPO - 7200)) ok                # aelterer Knoten, zwei Stunden voraus
lauf '{"bescheinigung_konsens":{"register_ab":9223372036854775807}}' $((REPO - 7200)) ok  # schlafend
lauf '{}' $((REPO - 600)) halt                                                    # nur zehn Minuten voraus
lauf '{}' $((REPO + 60)) halt                                                     # schon vorbei
lauf '{"bescheinigung_konsens":{"register_ab":1700000000}}' $((REPO - 7200)) halt # laufender Knoten nimmt sie schon an
lauf '{"bescheinigung_konsens":{"register_ab":"x"}}' $((REPO - 7200)) halt        # unlesbar
rm -f "$D/api/status"
if bash .github/neustart/b1b-stichtag-nicht-verpasst.sh "http://127.0.0.1:$PORT" 0 >/dev/null 2>&1; then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
