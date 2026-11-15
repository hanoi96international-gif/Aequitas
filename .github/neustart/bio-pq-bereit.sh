#!/usr/bin/env bash
# Haelt den Deploy an, wenn genesis.json im Repo den Stichtag der
# quantensicheren Bescheinigung (bescheinigung.pq_ab_unix, Stufe 1,
# bio_bescheinigung_pq.go) aendert und das nicht nachweislich sicher ist. Ab
# dem Stichtag zaehlt eine Registrierung nur mit gueltigen ML-DSA-65-
# Unterschriften von Coordinator und Vergleichsdienst -- unterschreiben sie
# falsch, nimmt die Kette keinen Menschen mehr an.
#
# Erlaubt, den Stichtag zu setzen oder zu verschieben, nur wenn:
#   - er mindestens PUFFER Sekunden in der Zukunft liegt,
#   - der bisherige Stichtag (falls gesetzt) noch nicht erreicht ist,
#   - der laufende Knoten seit seinem Start mindestens eine echte
#     Registrierung beobachtet hat, deren PQ-Unterschriften die strenge
#     Pruefung bestanden (pq_beobachtet.ok >= 1), und keine, die sie nicht
#     bestanden haette (pq_beobachtet.falsch = 0).
# Die Zaehler beginnen bei jedem Neustart bei 0: liegt zwischen der
# Registrierung und diesem Deploy ein Neustart, auf die naechste warten.
# Ausschalten (0) nach dem Stichtag: nie. Antwortet der Knoten nicht: anhalten.
# Aufruf: bio-pq-bereit.sh URL [JETZT_UNIX]
set -euo pipefail
PUFFER=1800
BASE=$1
JETZT=${2:-$(date -u +%s)}
REPO=$(jq -r '(.bescheinigung.pq_ab_unix // 0) | tostring' genesis.json 2>/dev/null) || { echo "::error::genesis.json unlesbar"; exit 1; }
[[ "$REPO" =~ ^(0|[1-9][0-9]{0,11})$ ]] || { echo "::error::bescheinigung.pq_ab_unix im Repo unlesbar: $REPO"; exit 1; }
STATUS=$(curl -sf -m 15 "$BASE/api/status") || { echo "::error::laufender Knoten antwortet nicht -- Deploy angehalten"; exit 1; }
lies() { printf '%s' "$STATUS" | jq -r "$1" 2>/dev/null; }
LIVE=$(lies '(.bescheinigung_konsens.pq_ab_unix // 0) | tostring') || { echo "::error::Antwort ist kein JSON -- Deploy angehalten"; exit 1; }
[[ "$LIVE" =~ ^(0|[1-9][0-9]{0,11})$ ]] || { echo "::error::pq_ab_unix des laufenden Knotens unlesbar: $LIVE"; exit 1; }
echo "Bescheinigung PQ: Stichtag Repo $REPO, laufend $LIVE, jetzt $JETZT"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
if [ "$LIVE" != 0 ] && [ "$JETZT" -ge "$LIVE" ]; then
  echo "::error::Die PQ-Pflicht gilt seit $LIVE; ein anderer Stichtag waere rueckwirkend."
  exit 1
fi
if [ "$REPO" = 0 ]; then
  echo "Stichtag wird vor dem Erreichen zurueckgenommen -- erlaubt"
  exit 0
fi
if [ "$REPO" -lt $((JETZT + PUFFER)) ]; then
  echo "::error::Neuer Stichtag $REPO liegt nicht mindestens $PUFFER s in der Zukunft."
  exit 1
fi
OK=$(lies '(.bescheinigung_konsens.pq_beobachtet.ok // -1) | tostring')
FALSCH=$(lies '(.bescheinigung_konsens.pq_beobachtet.falsch // -1) | tostring')
[[ "$OK" =~ ^[0-9]+$ ]] && [ "$OK" -ge 1 ] || { echo "::error::Noch keine echte Registrierung mit gueltigen PQ-Unterschriften beobachtet (seit dem letzten Neustart) -- warten."; exit 1; }
[ "$FALSCH" = 0 ] || { echo "::error::$FALSCH Registrierung(en) haetten die PQ-Pruefung NICHT bestanden -- Coordinator/Vergleichsdienst pruefen."; exit 1; }
echo "bereit: $OK gueltige PQ-Pruefungen, 0 falsche -- ausrollen erlaubt"
