#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/bio-pq-bereit.sh.
set -euo pipefail
cd "$(dirname "$0")/.."
SKRIPT="$PWD/.github/neustart/bio-pq-bereit.sh"
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/web/api" "$D/repo"
PORT=$((20000 + RANDOM % 20000))
(cd "$D/web" && python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) & PID=$!
# Erst weiter, wenn der Server antwortet (hoechstens 10 s) -- eine feste
# Sekunde reichte auf vollen CI-Runnern nicht: dann lief "nichts gesetzt"
# gegen einen stummen Server und hielt an (15.11.2026, zweimal).
: > "$D/web/bereit"
for _ in $(seq 1 50); do curl -sf -m 1 "http://127.0.0.1:$PORT/bereit" >/dev/null && break; sleep 0.2; done
curl -sf -m 1 "http://127.0.0.1:$PORT/bereit" >/dev/null || { echo "FEHLER: Testserver startet nicht"; exit 1; }
J=1800000000
gen() { printf '{"bescheinigung":{"ab_unix":1,"pq_ab_unix":%s}}' "$1" > "$D/repo/genesis.json"; }
st() { printf '{"bescheinigung_konsens":{"pq_ab_unix":%s,"pq_beobachtet":{"ok":%s,"falsch":%s,"ohne":0}}}' "$1" "$2" "$3" > "$D/web/api/status"; }
lauf() { # name erwartung
  if (cd "$D/repo" && bash "$SKRIPT" "http://127.0.0.1:$PORT" "$J") >/dev/null 2>&1; then e=ok; else e=halt; fi
  [ "$e" = "$2" ] || { echo "FEHLER: $1 -> $e, erwartet $2"; exit 1; }
  echo "ok: $1 -> $e"
}
gen 0;              st 0 0 0;                  lauf "nichts gesetzt" ok
gen $((J + 7200));  st 0 1 0;                  lauf "beobachtet gueltig, zwei Stunden voraus" ok
gen $((J + 7200));  st 0 0 0;                  lauf "noch keine Registrierung beobachtet" halt
gen $((J + 7200));  st 0 3 1;                  lauf "eine Registrierung haette nicht bestanden" halt
gen $((J + 600));   st 0 3 0;                  lauf "nur zehn Minuten voraus" halt
gen $((J + 7200));  st $((J - 60)) 3 0;        lauf "schon Pflicht, verschoben" halt
gen 0;              st $((J - 60)) 3 0;        lauf "schon Pflicht, ausgeschaltet" halt
gen 0;              st $((J + 3600)) 3 0;      lauf "vor dem Erreichen zurueckgenommen" ok
gen $((J - 60));    st $((J - 60)) 3 0;        lauf "gleicher Stichtag, schon Pflicht" ok
gen $((J + 7200));  printf '{"bescheinigung_konsens":{"pq_ab_unix":0}}' > "$D/web/api/status"; lauf "Knoten ohne Beobachtung (alter Stand)" halt
gen $((J + 7200));  printf 'kein json' > "$D/web/api/status"; lauf "kaputte Antwort" halt
printf '{"bescheinigung":{"pq_ab_unix":"1e9"}}' > "$D/repo/genesis.json"; st 0 3 0; lauf "Stichtag im Repo unlesbar" halt
rm -f "$D/web/api/status"; gen $((J + 7200)); lauf "ohne Antwort" halt
