#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/quanten-s-probe-v2-nicht-verpasst.sh
# mit einem kleinen HTTP-Server, der einen laufenden Knoten spielt. Zwei
# Teile: der Stand im Repo (heute schlafend, MaxInt64) und eine Kopie mit
# gesetztem Stichtag (wie nach der Freigabe des Betreibers).
set -euo pipefail
cd "$(dirname "$0")/.."
WAECHTER="$PWD/.github/neustart/quanten-s-probe-v2-nicht-verpasst.sh"
MAX=9223372036854775807
REPO=$(sed -n 's/^\tquantenSProbeV2AbUnix int64 = \([0-9]\{1,\}\)$/\1/p' x/humanity/keeper/quanten_s_probe_v2.go)
[ -n "$REPO" ] || { echo "FEHLER: quantenSProbeV2AbUnix nicht lesbar"; exit 1; }
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/api" "$D/repo/x/humanity/keeper"
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
lauf() { # verzeichnis status-inhalt jetzt erwartung
  printf '%s' "$2" > "$D/api/status"
  if (cd "$1" && bash "$WAECHTER" "http://127.0.0.1:$PORT" "$3" >/dev/null 2>&1); then e=ok; else e=halt; fi
  [ "$e" = "$4" ] || { echo "FEHLER: status=$2 jetzt=$3 -> $e, erwartet $4"; exit 1; }
  echo "ok: status=$2 jetzt=$3 -> $e"
}
J=1800000000
# Teil 1: das Repo schlaeft (heute).
[ "$REPO" = "$MAX" ] || echo "Hinweis: Stichtag im Repo gesetzt ($REPO)"
if [ "$REPO" = "$MAX" ]; then
  lauf . '{}' $J ok                                             # laufend schlafend (aelterer Stand)
  lauf . '{"quanten_w":{"ab":1794247200}}' $J ok                # W gesetzt, v2 schlafend
  lauf . "{\"quanten_w\":{\"probe_v2_ab\":$((J + 7200))}}" $J ok   # zurueck auf schlafend vor dem Start
  lauf . "{\"quanten_w\":{\"probe_v2_ab\":$((J - 60))}}" $J halt   # laufend schon aktiv: rueckwirkend
fi
# Teil 2: Kopie mit gesetztem Stichtag.
S=1900000000
printf '\tquantenSProbeV2AbUnix int64 = %s\n' "$S" > "$D/repo/x/humanity/keeper/quanten_s_probe_v2.go"
lauf "$D/repo" '{}' $((S - 7200)) ok                             # zwei Stunden voraus
lauf "$D/repo" '{"quanten_w":{}}' $((S - 7200)) ok               # laufend schlafend
lauf "$D/repo" "{\"quanten_w\":{\"probe_v2_ab\":$S}}" $((S + 999)) ok   # gleicher Stichtag
lauf "$D/repo" '{}' $((S - 600)) halt                            # nur zehn Minuten voraus
lauf "$D/repo" '{}' $((S + 60)) halt                             # schon vorbei
lauf "$D/repo" '{"quanten_w":{"probe_v2_ab":1700000000}}' $((S - 7200)) halt  # laufend schon aktiv
lauf "$D/repo" '{"quanten_w":{"probe_v2_ab":"x"}}' $((S - 7200)) halt         # unlesbar
lauf "$D/repo" 'kein json' $((S - 7200)) halt                                # kaputte Antwort
lauf "$D/repo" '{"quanten_w":{"probe_v2_ab":99999999999999999999}}' $((S - 7200)) halt  # riesig
lauf "$D/repo" '{"quanten_w":{"probe_v2_ab":"0001900000000"}}' $((S - 7200)) halt       # fuehrende Nullen
printf 'kaputt\n' > "$D/repo/x/humanity/keeper/quanten_s_probe_v2.go"
lauf "$D/repo" '{}' $((S - 7200)) halt                           # Repo unlesbar
rm -f "$D/api/status"
if (cd "$D/repo" && bash "$WAECHTER" "http://127.0.0.1:$PORT" 0 >/dev/null 2>&1); then echo "FEHLER: ohne Antwort erlaubt"; exit 1; fi
echo "ok: ohne Antwort -> halt"
