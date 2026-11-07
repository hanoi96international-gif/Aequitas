#!/usr/bin/env bash
# Haelt den Deploy an, wenn der Stichtag des Gleichgewichts im Repo
# (gleichgewichtAktivAbUnix, x/humanity/keeper/wirtschaft3.go) ein anderer ist
# als der des laufenden Knotens und nicht mehr mindestens PUFFER Sekunden in
# der Zukunft liegt. Gleicher Stichtag: nichts zu pruefen.
# Aufruf: stichtag-nicht-verpasst.sh URL [JETZT_UNIX]
set -euo pipefail
PUFFER=1800
BASE=$1
JETZT=${2:-$(date -u +%s)}
REPO=$(sed -n 's/^const gleichgewichtAktivAbUnix int64 = \([0-9]\{1,\}\)$/\1/p' x/humanity/keeper/wirtschaft3.go)
[ -n "$REPO" ] || { echo "::error::gleichgewichtAktivAbUnix im Repo nicht lesbar"; exit 1; }
LIVE=$(curl -s -m 15 "$BASE/api/wirtschaft/regeln" | jq -r '.gleichgewicht.aktiv_ab // empty' 2>/dev/null || true)
[ -n "$LIVE" ] || { echo "::error::Stichtag des laufenden Knotens nicht lesbar -- Deploy angehalten"; exit 1; }
echo "Stichtag Gleichgewicht: Repo $REPO, laufend $LIVE, jetzt $JETZT"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
if [ "$REPO" -lt $((JETZT + PUFFER)) ]; then
  echo "::error::Neuer Stichtag $REPO liegt nicht mindestens $PUFFER s in der Zukunft (jetzt $JETZT). Ausgerollt wuerde er Bloecke anders nachrechnen, als sie entstanden. Neuen Stichtag waehlen."
  exit 1
fi
if [ "$LIVE" -le "$JETZT" ]; then
  echo "::error::Der laufende Knoten wendet das Gleichgewicht schon an (seit $LIVE); ein anderer Stichtag waere ein Bruch der Regeln rueckwirkend."
  exit 1
fi
echo "neuer Stichtag liegt in der Zukunft -- ausrollen erlaubt"
