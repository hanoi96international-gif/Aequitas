#!/usr/bin/env bash
# Haelt den Deploy an, wenn genesis.json im Repo die Stichhoehe des PQ-Stempels
# (pq_stempel.ab_hoehe, pq_stempel.go) aendert und der laufende Knoten dafuer
# nicht bereit ist. Ab der Stichhoehe weist jeder Knoten einen Block auf jeder
# 60. Hoehe ohne gueltigen Stempel ab -- stempelt der Erzeuger dann nicht
# (kein Seed, falscher Schluessel in der Genesis), steht die Kette.
#
# Erlaubt, eine Stichhoehe zu setzen oder zu verschieben, nur wenn:
#   - sie mindestens PUFFER Bloecke ueber der laufenden Hoehe liegt,
#   - die bisherige Stichhoehe (falls gesetzt) noch nicht erreicht ist,
#   - der laufende Knoten selbst stempelt (pq.stempel.eigener),
#     mindestens MIN_OK Stempel gegen die Genesis geprueft hat und keinen
#     falschen (geprueft_falsch = 0),
#   - jeder Erzeuger aus /api/status (produzenten) einen Schluessel in der
#     Genesis des Repos hat.
# Ausschalten (0) nach Erreichen der Stichhoehe: nie (rueckwirkend schwaecher).
# Antwortet der Knoten nicht oder unlesbar: anhalten.
# Aufruf: pq-stempel-bereit.sh URL
set -euo pipefail
PUFFER=1800
MIN_OK=3
BASE=$1
REPO=$(jq -r '(.pq_stempel.ab_hoehe // 0) | tostring' genesis.json 2>/dev/null) || { echo "::error::genesis.json unlesbar"; exit 1; }
[[ "$REPO" =~ ^(0|[1-9][0-9]{0,12})$ ]] || { echo "::error::pq_stempel.ab_hoehe im Repo unlesbar: $REPO"; exit 1; }
STATUS=$(curl -sf -m 15 "$BASE/api/status") || { echo "::error::laufender Knoten antwortet nicht -- Deploy angehalten"; exit 1; }
lies() { printf '%s' "$STATUS" | jq -r "$1" 2>/dev/null; }
LIVE=$(lies '(.pq.stempel.ab_hoehe // 0) | tostring') || { echo "::error::Antwort ist kein JSON -- Deploy angehalten"; exit 1; }
HOEHE=$(lies '(.height // "x") | tostring')
[[ "$LIVE" =~ ^(0|[1-9][0-9]{0,12})$ ]] || { echo "::error::Stichhoehe des laufenden Knotens unlesbar: $LIVE"; exit 1; }
[[ "$HOEHE" =~ ^[1-9][0-9]{0,12}$ ]] || { echo "::error::Hoehe des laufenden Knotens unlesbar: $HOEHE"; exit 1; }
echo "PQ-Stempel: Stichhoehe Repo $REPO, laufend $LIVE, Hoehe $HOEHE"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
if [ "$LIVE" != 0 ] && [ "$HOEHE" -ge "$LIVE" ]; then
  echo "::error::Der PQ-Stempel ist seit Hoehe $LIVE Pflicht; eine andere Stichhoehe waere rueckwirkend."
  exit 1
fi
if [ "$REPO" = 0 ]; then
  echo "Stichhoehe wird vor dem Erreichen zurueckgenommen -- erlaubt"
  exit 0
fi
if [ "$REPO" -lt $((HOEHE + PUFFER)) ]; then
  echo "::error::Neue Stichhoehe $REPO liegt nicht mindestens $PUFFER Bloecke ueber $HOEHE."
  exit 1
fi
EIGEN=$(lies '(.pq.stempel.eigener // false) | tostring')
OK=$(lies '(.pq.stempel.geprueft_ok // -1) | tostring')
FALSCH=$(lies '(.pq.stempel.geprueft_falsch // -1) | tostring')
[ "$EIGEN" = true ] || { echo "::error::Der laufende Knoten stempelt nicht (kein ERZEUGER_MLDSA_SEED) -- erst erzeuger-pq-schluessel-anlegen.yml."; exit 1; }
[[ "$OK" =~ ^[0-9]+$ ]] && [ "$OK" -ge "$MIN_OK" ] || { echo "::error::Erst $OK Stempel gegen die Genesis geprueft (mindestens $MIN_OK) -- warten."; exit 1; }
[ "$FALSCH" = 0 ] || { echo "::error::$FALSCH Stempel passten NICHT zur Genesis -- Schluessel in genesis.json pruefen."; exit 1; }
FEHLT=$(printf '%s' "$STATUS" | jq -r '.produzenten // [] | .[] | ascii_downcase' 2>/dev/null | while read -r p; do
  jq -e --arg p "$p" '.pq_stempel.schluessel[$p] // empty' genesis.json >/dev/null 2>&1 || echo "$p"
done)
N=$(lies '(.produzenten // []) | length')
[ "${N:-0}" -ge 1 ] || { echo "::error::Keine Erzeugerliste im Status -- nicht pruefbar."; exit 1; }
[ -z "$FEHLT" ] || { echo "::error::Erzeuger ohne PQ-Schluessel in genesis.json: $FEHLT"; exit 1; }
echo "bereit: $OK Stempel geprueft, 0 falsch, alle $N Erzeuger mit Schluessel -- ausrollen erlaubt"
