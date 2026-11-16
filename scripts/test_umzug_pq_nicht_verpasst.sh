#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/umzug-pq-nicht-verpasst.sh
# (Stichtag der PQ-Unterschriften am Umzug, umzug_pq.go) mit einem kleinen
# HTTP-Server, der einen laufenden Knoten spielt, und einem Schein-Repo je
# Stichtag im Code (schlafend = MaxInt64 oder eine Zeit).
set -euo pipefail
cd "$(dirname "$0")/.."
WACHE="$PWD/.github/neustart/umzug-pq-nicht-verpasst.sh"
ECHT=$(sed -n 's/^\tumzugPQAbUnix int64 = \([0-9]*\)\( \/\/.*\)\{0,1\}$/\1/p' x/humanity/keeper/umzug_pq.go)
[ -n "$ECHT" ] || { echo "FEHLER: umzugPQAbUnix im Repo nicht lesbar"; exit 1; }
MAX=9223372036854775807
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api" "$D/repo/x/humanity/keeper"
PORT=$((20000 + RANDOM % 20000))
# Testserver starten und warten, bis er antwortet. Ein starres "sleep 1"
# kippte auf ausgelasteten Runnern (Pruefung meldete "halt" statt "ok");
# ein belegter Port nimmt einen anderen.
for _versuch in 1 2 3 4 5; do
  PORT=$((20000 + RANDOM % 20000))
  (cd "$D" && exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) & PID=$!
  for _i in $(seq 1 150); do
    curl -s -o /dev/null "http://127.0.0.1:$PORT/" && break 2
    kill -0 "$PID" 2>/dev/null || break
    sleep 0.1
  done
  kill "$PID" 2>/dev/null || true
done
curl -s -o /dev/null "http://127.0.0.1:$PORT/" || { echo "FEHLER: Testserver startet nicht"; exit 1; }
lauf() { # code-stichtag status-inhalt jetzt erwartung
  printf 'package keeper\n\nconst (\n\tumzugPQAbUnix int64 = %s // Test\n)\n' "$1" > "$D/repo/x/humanity/keeper/umzug_pq.go"
  printf '%s' "$2" > "$D/api/status"
  if (cd "$D/repo" && bash "$WACHE" "http://127.0.0.1:$PORT" "$3" >/dev/null 2>&1); then e=ok; else e=halt; fi
  [ "$e" = "$4" ] || { echo "FEHLER: code=$1 status=$2 jetzt=$3 -> $e, erwartet $4"; exit 1; }
  echo "ok: code=$1 status=$2 jetzt=$3 -> $e"
}
T=1800000000
lauf $MAX '{"umzug_pq":{}}' $T ok                        # beide schlafend
lauf $MAX '{}' $T ok                                       # aelterer Knoten, Code schlafend
lauf $T "{\"umzug_pq\":{\"ab\":$T}}" $((T + 99999)) ok    # gleicher Stichtag: immer erlaubt
lauf $T '{"umzug_pq":{}}' $((T - 7200)) ok               # neu gesetzt, zwei Stunden voraus
lauf $T '{"umzug_pq":{}}' $((T - 600)) halt              # nur zehn Minuten voraus
lauf $T '{"umzug_pq":{}}' $((T + 60)) halt               # schon vorbei
lauf $MAX "{\"umzug_pq\":{\"ab\":$((T - 10))}}" $T halt  # laufend schon scharf, Code will zurueck
lauf $T '{"umzug_pq":{"ab":"x"}}' $((T - 7200)) halt     # unlesbar
lauf $T 'kein json' $((T - 7200)) halt                     # kaputte Antwort
lauf $T '{"umzug_pq":{"ab":"0001800000000"}}' $((T - 7200)) halt  # fuehrende Nullen
lauf 'math.MaxInt64' '{}' $T halt                          # Konstante nicht als Zahl: nicht lesbar
rm -f "$D/api/status"
if (cd "$D/repo" && bash "$WACHE" "http://127.0.0.1:$PORT" 0 >/dev/null 2>&1); then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
echo "Stichtag im Repo: $ECHT"
