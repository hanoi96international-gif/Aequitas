#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/pruefer-dienst-pq-nicht-verpasst.sh
# (Stichtag der PQ-Schluessel der Pruefer-Dienste, pruefer_dienst_pq.go) mit einem kleinen
# HTTP-Server, der einen laufenden Knoten spielt, und einem Schein-Repo je
# Stichtag im Code (schlafend = MaxInt64 oder eine Zeit).
set -euo pipefail
cd "$(dirname "$0")/.."
WACHE="$PWD/.github/neustart/pruefer-dienst-pq-nicht-verpasst.sh"
ECHT=$(sed -n 's/^\tprueferDienstPQAbUnix int64 = \([0-9]*\)\( \/\/.*\)\{0,1\}$/\1/p' x/humanity/keeper/pruefer_dienst_pq.go)
[ -n "$ECHT" ] || { echo "FEHLER: prueferDienstPQAbUnix im Repo nicht lesbar"; exit 1; }
MAX=9223372036854775807
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api" "$D/repo/x/humanity/keeper"
PORT=$((20000 + RANDOM % 20000))
(cd "$D" && python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) & PID=$!
sleep 1
lauf() { # code-stichtag status-inhalt jetzt erwartung
  printf 'package keeper\n\nconst (\n\tprueferDienstPQAbUnix int64 = %s // Test\n)\n' "$1" > "$D/repo/x/humanity/keeper/pruefer_dienst_pq.go"
  printf '%s' "$2" > "$D/api/status"
  if (cd "$D/repo" && bash "$WACHE" "http://127.0.0.1:$PORT" "$3" >/dev/null 2>&1); then e=ok; else e=halt; fi
  [ "$e" = "$4" ] || { echo "FEHLER: code=$1 status=$2 jetzt=$3 -> $e, erwartet $4"; exit 1; }
  echo "ok: code=$1 status=$2 jetzt=$3 -> $e"
}
T=1800000000
lauf $MAX '{"pruefer_dienst_pq":{}}' $T ok                        # beide schlafend
lauf $MAX '{}' $T ok                                       # aelterer Knoten, Code schlafend
lauf $T "{\"pruefer_dienst_pq\":{\"ab\":$T}}" $((T + 99999)) ok    # gleicher Stichtag: immer erlaubt
lauf $T '{"pruefer_dienst_pq":{}}' $((T - 7200)) ok               # neu gesetzt, zwei Stunden voraus
lauf $T '{"pruefer_dienst_pq":{}}' $((T - 600)) halt              # nur zehn Minuten voraus
lauf $T '{"pruefer_dienst_pq":{}}' $((T + 60)) halt               # schon vorbei
lauf $MAX "{\"pruefer_dienst_pq\":{\"ab\":$((T - 10))}}" $T halt  # laufend schon scharf, Code will zurueck
lauf $T '{"pruefer_dienst_pq":{"ab":"x"}}' $((T - 7200)) halt     # unlesbar
lauf $T 'kein json' $((T - 7200)) halt                     # kaputte Antwort
lauf $T '{"pruefer_dienst_pq":{"ab":"0001800000000"}}' $((T - 7200)) halt  # fuehrende Nullen
lauf 'math.MaxInt64' '{}' $T halt                          # Konstante nicht als Zahl: nicht lesbar
rm -f "$D/api/status"
if (cd "$D/repo" && bash "$WACHE" "http://127.0.0.1:$PORT" 0 >/dev/null 2>&1); then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
echo "Stichtag im Repo: $ECHT"
