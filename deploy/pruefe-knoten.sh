#!/usr/bin/env bash
# Antwortet der Knoten, und steigt seine Hoehe? (Aufruf: pruefe-knoten.sh URL)
# Gleiche Pruefung wie in den frueheren deploy-contabo*.yml.
set -uo pipefail
BASE="$1"
up=""
for i in $(seq 1 30); do
  code=$(curl -s -m 10 -o /dev/null -w '%{http_code}' "$BASE/api/health/combined" || echo 000)
  [ "$code" = 200 ] && { up=yes; echo "antwortet (Versuch $i)"; break; }
  echo "warte auf Antwort ($i/30, zuletzt HTTP $code)"; sleep 10
done
[ -n "$up" ] || { echo "FEHLER: kein 200 von $BASE/api/health/combined in 5 Minuten"; exit 1; }
hoehe() { curl -s -m 25 "$BASE/api/status" | jq -r '.height // empty' 2>/dev/null; }
h0=""
for i in $(seq 1 21); do
  h=$(hoehe || true)
  case "$h" in ''|*[!0-9]*) echo "Probe $i: keine Hoehe";;
    *) if [ -z "$h0" ]; then h0=$h; echo "Probe $i: Ausgangshoehe $h0"
       elif [ "$h" -gt "$h0" ]; then echo "Probe $i: $h0 -> $h -- laeuft"; exit 0
       else echo "Probe $i: noch $h"; fi;;
  esac
  sleep 30
done
echo "FEHLER: Hoehe steigt nicht (Ausgang ${h0:-keine})"; exit 1
