#!/usr/bin/env bash
# Haelt den Deploy an, wenn der laufende Knoten die Quanten-Noten noch im
# alten Format fuehrt (ganzes Register in chain_config, vor QG3) und schon
# Noten hat. Der neue Knoten liest Noten nur aus der Tabelle quanten_noten
# und startet nicht, wenn noch welche in chain_config stehen
# (quantenGeldAusDB). Ein Knoten mit quanten.noten_tabelle = true fuehrt
# schon das neue Format. Ohne Antwort, mit kaputtem JSON oder unlesbarer
# Zahl: anhalten.
# Aufruf: quantengeld-noten-tabelle.sh URL
set -euo pipefail
BASE=$1
STATUS=$(curl -sf -m 15 "$BASE/api/status") || { echo "::error::laufender Knoten antwortet nicht -- Deploy angehalten"; exit 1; }
NEU=$(printf '%s' "$STATUS" | jq -r '(.quanten // {}) | (.noten_tabelle // false) | tostring' 2>/dev/null) || { echo "::error::Antwort des laufenden Knotens ist kein JSON -- Deploy angehalten"; exit 1; }
if [ "$NEU" = true ]; then
  echo "laufender Knoten fuehrt die Noten schon je Zeile"
  exit 0
fi
[ "$NEU" = false ] || { echo "::error::quanten.noten_tabelle unlesbar: $NEU"; exit 1; }
STAND=$(curl -sf -m 15 "$BASE/api/quanten/stand") || { echo "::error::/api/quanten/stand antwortet nicht -- Deploy angehalten"; exit 1; }
NOTEN=$(printf '%s' "$STAND" | jq -r 'if (.noten | type) == "number" then (.noten | tostring) else "x" end' 2>/dev/null) || { echo "::error::/api/quanten/stand ist kein JSON -- Deploy angehalten"; exit 1; }
[[ "$NOTEN" =~ ^(0|[1-9][0-9]{0,5})$ ]] || { echo "::error::Zahl der Noten unlesbar: $NOTEN -- Deploy angehalten"; exit 1; }
echo "Noten im alten Format: $NOTEN"
if [ "$NOTEN" != 0 ]; then
  echo "::error::Der laufende Knoten hat $NOTEN Quanten-Noten im alten Format (chain_config). Der neue Knoten liest sie dort nicht mehr -- erst umziehen, dann ausrollen."
  exit 1
fi
echo "keine Note -- Wechsel auf die Tabelle erlaubt"
