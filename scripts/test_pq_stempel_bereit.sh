#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer .github/neustart/pq-stempel-bereit.sh mit
# einem kleinen HTTP-Server, der einen laufenden Knoten spielt, und einer
# eigenen genesis.json je Fall.
set -euo pipefail
cd "$(dirname "$0")/.."
SKRIPT="$PWD/.github/neustart/pq-stempel-bereit.sh"
D=$(mktemp -d); trap 'kill $PID 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/web/api" "$D/repo"
PORT=$((20000 + RANDOM % 20000))
# Testserver starten und warten, bis er antwortet (ein starres "sleep 1"
# kippte auf ausgelasteten Runnern); ein belegter Port nimmt einen anderen.
for _versuch in 1 2 3 4 5; do
  PORT=$((20000 + RANDOM % 20000))
  (cd "$D/web" && exec python3 -m http.server "$PORT" --bind 127.0.0.1 >/dev/null 2>&1) & PID=$!
  for _i in $(seq 1 150); do
    curl -s -o /dev/null "http://127.0.0.1:$PORT/" && break 2
    kill -0 "$PID" 2>/dev/null || break
    sleep 0.1
  done
  kill "$PID" 2>/dev/null || true
done
curl -s -o /dev/null "http://127.0.0.1:$PORT/" || { echo "FEHLER: Testserver startet nicht"; exit 1; }
E=0x00000000000000000000000000000000000000e1
K=$(printf 'ab%.0s' $(seq 1 1952))
gen() { # ab_hoehe [schluessel-adresse]
  local a=${2:-$E}
  printf '{"chain_id":"x","pq_stempel":{"ab_hoehe":%s,"schluessel":{"%s":"%s"}}}' "$1" "$a" "$K" > "$D/repo/genesis.json"
}
st() { # ab hoehe eigener ok falsch [produzent]
  printf '{"height":%s,"produzenten":["%s"],"pq":{"ueberweisung_ab":1,"stempel":{"ab_hoehe":%s,"eigener":%s,"geprueft_ok":%s,"geprueft_falsch":%s}}}' \
    "$2" "${6:-$E}" "$1" "$3" "$4" "$5" > "$D/web/api/status"
}
lauf() { # name erwartung
  if (cd "$D/repo" && bash "$SKRIPT" "http://127.0.0.1:$PORT") >/dev/null 2>&1; then e=ok; else e=halt; fi
  [ "$e" = "$2" ] || { echo "FEHLER: $1 -> $e, erwartet $2"; exit 1; }
  echo "ok: $1 -> $e"
}
gen 0;       st 0 100000 false 0 0;         lauf "nichts gesetzt, nichts geaendert" ok
gen 200000;  st 0 100000 true 5 0;          lauf "bereit, Stichhoehe weit voraus" ok
gen 200000;  st 0 199000 true 5 0;          lauf "nur 1.000 Bloecke voraus" halt
gen 200000;  st 0 100000 false 5 0;         lauf "Erzeuger stempelt nicht" halt
gen 200000;  st 0 100000 true 2 0;          lauf "zu wenige gepruefte Stempel" halt
gen 200000;  st 0 100000 true 50 1;         lauf "ein falscher Stempel" halt
gen 200000 0x00000000000000000000000000000000000000e2; st 0 100000 true 5 0; lauf "Erzeuger ohne Schluessel in der Genesis" halt
gen 200000;  st 0 100000 true 5 0 0x00000000000000000000000000000000000000E1; lauf "Erzeuger gross geschrieben" ok
gen 300000;  st 200000 250000 true 5 0;     lauf "schon Pflicht, Stichhoehe verschoben" halt
gen 0;       st 200000 250000 true 5 0;     lauf "schon Pflicht, ausgeschaltet" halt
gen 0;       st 200000 150000 true 5 0;     lauf "vor dem Erreichen zurueckgenommen" ok
gen 200000;  st 200000 250000 true 5 0;     lauf "gleiche Stichhoehe, schon Pflicht" ok
gen 200000;  printf 'kein json' > "$D/web/api/status"; lauf "kaputte Antwort" halt
gen 200000;  printf '{"height":100000,"pq":{"stempel":{"ab_hoehe":"x"}}}' > "$D/web/api/status"; lauf "Stichhoehe unlesbar" halt
gen 200000;  printf '{"height":100000,"pq":{"stempel":{"ab_hoehe":0,"eigener":true,"geprueft_ok":5,"geprueft_falsch":0}}}' > "$D/web/api/status"; lauf "keine Erzeugerliste" halt
printf '{"pq_stempel":{"ab_hoehe":"1e9"}}' > "$D/repo/genesis.json"; st 0 100000 true 5 0; lauf "Stichhoehe im Repo unlesbar" halt
gen 3600;    st 3600 0 true 0 0;            lauf "frisch neu gestartet (Hoehe 0), Stichhoehe unveraendert" ok
gen 3600;    st 0 0 true 0 0;               lauf "frisch neu gestartet, neue Stichhoehe ohne gepruefte Stempel" halt
gen 3600;    printf '{"pq":{"stempel":{"ab_hoehe":3600}}}' > "$D/web/api/status"; lauf "Hoehe fehlt" halt
gen 3600;    printf '{"height":"null","pq":{"stempel":{"ab_hoehe":3600}}}' > "$D/web/api/status"; lauf "Hoehe keine Zahl" halt
rm -f "$D/web/api/status"; gen 200000; lauf "ohne Antwort" halt
