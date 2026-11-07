#!/usr/bin/env bash
# Haelt den Deploy an, wenn der Stichtag des Umzugs im Repo (umzugAbUnix,
# x/humanity/keeper/umzug.go) ein anderer ist als der des laufenden Knotens
# und nicht mehr mindestens PUFFER Sekunden in der Zukunft liegt. Ein Knoten
# ohne umzug.aktiv_ab (aelterer Stand) gilt als schlafend. Antwortet der
# Knoten gar nicht: anhalten.
# Aufruf: umzug-stichtag-nicht-verpasst.sh URL [JETZT_UNIX]
set -euo pipefail
PUFFER=1800
MAX=9223372036854775807
BASE=$1
JETZT=${2:-$(date -u +%s)}
ROH=$(sed -n 's/^\tumzugAbUnix int64 = \([0-9]*\)$/\1/p' x/humanity/keeper/umzug.go)
case "$ROH" in
  *[!0-9]*|"") echo "::error::umzugAbUnix im Repo nicht lesbar"; exit 1 ;;
  *) REPO=$ROH ;;
esac
STATUS=$(curl -sf -m 15 "$BASE/api/status") || { echo "::error::laufender Knoten antwortet nicht -- Deploy angehalten"; exit 1; }
LIVE=$(printf '%s' "$STATUS" | jq -r '(.umzug // {}) | (.aktiv_ab // "schlafend") | tostring' 2>/dev/null || echo schlafend)
[ "$LIVE" = schlafend ] && LIVE=$MAX
[[ "$LIVE" =~ ^[0-9]+$ ]] || { echo "::error::Stichtag des Umzugs im laufenden Knoten unlesbar: $LIVE"; exit 1; }
echo "Stichtag Umzug: Repo $REPO, laufend $LIVE, jetzt $JETZT"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
if [ "$REPO" -lt $((JETZT + PUFFER)) ]; then
  echo "::error::Neuer Stichtag $REPO liegt nicht mindestens $PUFFER s in der Zukunft (jetzt $JETZT). Neuen Stichtag waehlen."
  exit 1
fi
if [ "$LIVE" -le "$JETZT" ]; then
  echo "::error::Der laufende Knoten nimmt Umzuege schon an (seit $LIVE); ein anderer Stichtag waere rueckwirkend."
  exit 1
fi
echo "neuer Stichtag liegt in der Zukunft -- ausrollen erlaubt"
