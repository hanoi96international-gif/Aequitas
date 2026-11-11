#!/usr/bin/env bash
# Haelt den Deploy an, wenn der Stichtag des Pruefer-Registers im Repo
# (verifierRegisterAbUnix, x/humanity/keeper/verifier_register.go) ein anderer
# ist als der des laufenden Knotens und nicht mehr mindestens PUFFER Sekunden
# in der Zukunft liegt. Ein Knoten ohne /api/verifier (aelterer Stand) gilt
# als schlafend. Antwortet der Knoten gar nicht: anhalten.
# Aufruf: verifier-stichtag-nicht-verpasst.sh URL [JETZT_UNIX]
set -euo pipefail
PUFFER=1800
MAX=9223372036854775807
BASE=$1
JETZT=${2:-$(date -u +%s)}
ROH=$(sed -n 's/^const verifierRegisterAbUnix int64 = \(.*\)$/\1/p' x/humanity/keeper/verifier_register.go)
case "$ROH" in
  math.MaxInt64) REPO=$MAX ;;
  *[!0-9]*|"") echo "::error::verifierRegisterAbUnix im Repo nicht lesbar"; exit 1 ;;
  *) REPO=$ROH ;;
esac
curl -sf -m 15 "$BASE/api/status" >/dev/null || { echo "::error::laufender Knoten antwortet nicht -- Deploy angehalten"; exit 1; }
# Kaputtes JSON haelt an (nicht "schlafend"); ein JSON-Objekt ohne "ab"
# (aelterer Knoten, 404 mit Fehlerobjekt) gilt als schlafend.
VERIFIER=$(curl -s -m 15 "$BASE/api/verifier") || { echo "::error::/api/verifier antwortet nicht -- Deploy angehalten"; exit 1; }
LIVE=$(printf '%s' "$VERIFIER" | jq -r 'if type == "object" then (.ab // "schlafend") else "schlafend" end | tostring' 2>/dev/null) || { echo "::error::Antwort von /api/verifier ist kein JSON -- Deploy angehalten"; exit 1; }
[ "$LIVE" = schlafend ] && LIVE=$MAX
[[ "$LIVE" =~ ^[1-9][0-9]{0,11}$ || "$LIVE" = "$MAX" ]] || { echo "::error::Stichtag des laufenden Knotens unlesbar: $LIVE"; exit 1; }
echo "Stichtag Pruefer-Register: Repo $REPO, laufend $LIVE, jetzt $JETZT"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
if [ "$REPO" -lt $((JETZT + PUFFER)) ]; then
  echo "::error::Neuer Stichtag $REPO liegt nicht mindestens $PUFFER s in der Zukunft (jetzt $JETZT). Neuen Stichtag waehlen."
  exit 1
fi
if [ "$LIVE" -le "$JETZT" ]; then
  echo "::error::Der laufende Knoten fuehrt das Register schon (seit $LIVE); ein anderer Stichtag waere rueckwirkend."
  exit 1
fi
echo "neuer Stichtag liegt in der Zukunft -- ausrollen erlaubt"
