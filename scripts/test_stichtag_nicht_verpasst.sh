#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/stichtag-nicht-verpasst.sh
# mit einem kleinen HTTP-Server, der einen laufenden Knoten spielt.
set -euo pipefail
cd "$(dirname "$0")/.."
REPO=$(sed -n 's/^const gleichgewichtAktivAbUnix int64 = \([0-9]\{1,\}\)$/\1/p' x/humanity/keeper/wirtschaft3.go)
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api/wirtschaft"
PORT=$((20000 + RANDOM % 20000))
(cd "$D" && python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) & PID=$!
sleep 1
lauf() { # live jetzt erwartung
  printf '{"gleichgewicht":{"aktiv_ab":%s}}' "$1" > "$D/api/wirtschaft/regeln"
  if bash .github/neustart/stichtag-nicht-verpasst.sh "http://127.0.0.1:$PORT" "$2" >/dev/null 2>&1; then e=ok; else e=halt; fi
  [ "$e" = "$3" ] || { echo "FEHLER: live=$1 jetzt=$2 -> $e, erwartet $3"; exit 1; }
  echo "ok: live=$1 jetzt=$2 -> $e"
}
lauf "$REPO" $((REPO + 999999)) ok                 # gleicher Stichtag: immer erlaubt
lauf 4102444800 $((REPO - 7200)) ok                # neu, zwei Stunden voraus
lauf 4102444800 $((REPO - 600)) halt               # neu, nur zehn Minuten voraus
lauf 4102444800 $((REPO + 60)) halt                # neu, schon vorbei
lauf 1700000000 $((REPO - 7200)) halt              # laufender Knoten schon im Gleichgewicht
rm -f "$D/api/wirtschaft/regeln"
if bash .github/neustart/stichtag-nicht-verpasst.sh "http://127.0.0.1:$PORT" 0 >/dev/null 2>&1; then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
