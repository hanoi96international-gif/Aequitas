#!/usr/bin/env bash
# Haelt den Deploy an, wenn der Stichtag der Bescheinigung im Konsens (K1,
# genesis.json bescheinigung.ab_unix) ein anderer ist als der des laufenden
# Knotens und nicht mehr mindestens PUFFER Sekunden in der Zukunft liegt.
# Sonst entstuenden zwischen Stichtag und Deploy Bloecke ohne Pruefung, die
# jeder Knoten mit dem neuen Stand spaeter anders nachrechnet. Gleicher
# Stichtag: nichts zu pruefen.
# Aufruf: k1-stichtag-nicht-verpasst.sh URL [JETZT_UNIX]
set -euo pipefail
PUFFER=1800
BASE=$1
JETZT=${2:-$(date -u +%s)}
REPO=$(jq -r '.bescheinigung.ab_unix // 0' genesis.json)
[[ "$REPO" =~ ^[0-9]+$ ]] || { echo "::error::bescheinigung.ab_unix in genesis.json nicht lesbar"; exit 1; }
LIVE=$(curl -s -m 15 "$BASE/api/status" | jq -r '.bescheinigung_konsens.ab_unix // empty' 2>/dev/null || true)
[[ "$LIVE" =~ ^[0-9]+$ ]] || { echo "::error::K1-Stichtag des laufenden Knotens nicht lesbar -- Deploy angehalten"; exit 1; }
echo "Stichtag K1: Repo $REPO, laufend $LIVE, jetzt $JETZT"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
if [ "$REPO" -lt $((JETZT + PUFFER)) ]; then
  echo "::error::Neuer K1-Stichtag $REPO liegt nicht mindestens $PUFFER s in der Zukunft (jetzt $JETZT). Neuen Stichtag waehlen."
  exit 1
fi
if [ "$LIVE" -gt 0 ] && [ "$LIVE" -le "$JETZT" ]; then
  echo "::error::Der laufende Knoten prueft K1 schon (seit $LIVE); ein anderer Stichtag waere rueckwirkend."
  exit 1
fi
echo "neuer Stichtag liegt in der Zukunft -- ausrollen erlaubt"
