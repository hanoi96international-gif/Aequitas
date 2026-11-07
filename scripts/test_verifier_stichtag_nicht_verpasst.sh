#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/verifier-stichtag-nicht-verpasst.sh
# mit einem kleinen HTTP-Server, der einen laufenden Knoten spielt.
set -euo pipefail
cd "$(dirname "$0")/.."
REPO=$(sed -n 's/^const verifierRegisterAbUnix int64 = \([0-9]\{1,\}\)$/\1/p' x/humanity/keeper/verifier_register.go)
[ -n "$REPO" ] || { echo "Stichtag schlafend -- nichts zu testen"; exit 0; }
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api"
echo '{}' > "$D/api/status"
PORT=$((20000 + RANDOM % 20000))
(cd "$D" && python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) & PID=$!
sleep 1
lauf() { # inhalt jetzt erwartung
  if [ -n "$1" ]; then printf '%s' "$1" > "$D/api/verifier"; else rm -f "$D/api/verifier"; fi
  if bash .github/neustart/verifier-stichtag-nicht-verpasst.sh "http://127.0.0.1:$PORT" "$2" >/dev/null 2>&1; then e=ok; else e=halt; fi
  [ "$e" = "$3" ] || { echo "FEHLER: live=$1 jetzt=$2 -> $e, erwartet $3"; exit 1; }
  echo "ok: live=${1:-(kein Endpunkt)} jetzt=$2 -> $e"
}
lauf "{\"ab\":$REPO}" $((REPO + 999999)) ok           # gleicher Stichtag: immer erlaubt
lauf '' $((REPO - 7200)) ok                            # aelterer Knoten ohne Endpunkt, zwei Stunden voraus
lauf '{"aktiv":false}' $((REPO - 7200)) ok             # schlafend, zwei Stunden voraus
lauf '' $((REPO - 600)) halt                           # nur zehn Minuten voraus
lauf '' $((REPO + 60)) halt                            # schon vorbei
lauf '{"ab":1700000000}' $((REPO - 7200)) halt         # laufender Knoten fuehrt es schon
rm -f "$D/api/status"
if bash .github/neustart/verifier-stichtag-nicht-verpasst.sh "http://127.0.0.1:$PORT" 0 >/dev/null 2>&1; then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
