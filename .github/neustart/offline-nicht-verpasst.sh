#!/usr/bin/env bash
# Haelt den Deploy an, wenn der Stichtag des Offline-Bargelds im Repo
# (offlineAbUnix, x/humanity/keeper/offline_bargeld.go) ein anderer ist als der
# des laufenden Knotens und nicht mehr mindestens PUFFER Sekunden in der
# Zukunft liegt. Ein Knoten ohne offline.ab gilt als schlafend. Antwortet der
# Knoten gar nicht: anhalten.
#
# Dazu, einmalig: kennt der laufende Knoten das Offline-Bargeld noch gar nicht
# (kein "offline" im Status), muss der Offline-Topf leer sein. Mit diesem
# Stand wird er ein Protokoll-Topf (kein Umlauf, keine Vermoegensgrenze); ein
# Guthaben, das jemand vorher dorthin ueberwiesen haette, rechnete der neue
# Knoten anders als der alte.
# Aufruf: offline-nicht-verpasst.sh URL [JETZT_UNIX]
set -euo pipefail
PUFFER=1800
MAX=9223372036854775807
TOPF=0x5b83eeed08570c5d3552d649481a60aa6fb60097 # SHA-256("aequitas-offline-topf-v1"), letzte 20 Byte
BASE=$1
JETZT=${2:-$(date -u +%s)}
ROH=$(sed -n 's/^\tofflineAbUnix int64 = \([0-9]*\)$/\1/p' x/humanity/keeper/offline_bargeld.go)
case "$ROH" in
  *[!0-9]*|"") echo "::error::offlineAbUnix im Repo nicht lesbar"; exit 1 ;;
  *) REPO=$ROH ;;
esac
STATUS=$(curl -sf -m 15 "$BASE/api/status") || { echo "::error::laufender Knoten antwortet nicht -- Deploy angehalten"; exit 1; }
# Kaputtes JSON haelt an (nicht "schlafend"); nur ein fehlendes Feld gilt
# als aelterer Stand.
KENNT=$(printf '%s' "$STATUS" | jq -r 'has("offline")' 2>/dev/null) || { echo "::error::Antwort des laufenden Knotens ist kein JSON -- Deploy angehalten"; exit 1; }
if [ "$KENNT" != true ]; then
  BAL=$(curl -sf -m 15 "$BASE/api/balance?wallet=$TOPF" | jq -r '.balance | tostring' 2>/dev/null) || { echo "::error::Guthaben des Offline-Topfs nicht lesbar -- Deploy angehalten"; exit 1; }
  [[ "$BAL" =~ ^0(\.0+)?$ ]] || { echo "::error::Offline-Topf $TOPF hat Guthaben ($BAL) -- erst klaeren, dann ausrollen"; exit 1; }
  echo "Offline-Topf leer"
fi
LIVE=$(printf '%s' "$STATUS" | jq -r '(.offline // {}) | (.ab // "schlafend") | tostring' 2>/dev/null) || { echo "::error::Antwort des laufenden Knotens ist kein JSON -- Deploy angehalten"; exit 1; }
[ "$LIVE" = schlafend ] && LIVE=$MAX
[[ "$LIVE" =~ ^[1-9][0-9]{0,11}$ || "$LIVE" = "$MAX" ]] || { echo "::error::Stichtag des Offline-Bargelds im laufenden Knoten unlesbar: $LIVE"; exit 1; }
echo "Stichtag Offline-Bargeld: Repo $REPO, laufend $LIVE, jetzt $JETZT"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
if [ "$REPO" -lt $((JETZT + PUFFER)) ]; then
  echo "::error::Neuer Stichtag $REPO liegt nicht mindestens $PUFFER s in der Zukunft (jetzt $JETZT). Neuen Stichtag waehlen."
  exit 1
fi
if [ "$LIVE" -le "$JETZT" ]; then
  echo "::error::Der laufende Knoten nimmt Offline-Bargeld schon an (seit $LIVE); ein anderer Stichtag waere rueckwirkend."
  exit 1
fi
echo "neuer Stichtag liegt in der Zukunft -- ausrollen erlaubt"
