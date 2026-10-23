#!/usr/bin/env bash
# C1 zurueck auf das Image, das vor dem letzten Deploy lief
# (aequitas-node:vorher, gesetzt von deploy/deploy-c1.sh).
# Laeuft ueber .github/workflows/deploy-c1.yml, wenn das Ausrollen
# scheitert oder die Hoehe danach nicht steigt.
#
# Daten bleiben, wie sie sind: Postgres und WAL liegen in Volumes, und das
# vorige Image startet auf dem Schema, das das neue angelegt hat. Das traegt,
# solange eine Aenderung das Schema nur erweitert (CREATE/ADD ... IF NOT
# EXISTS, Indizes). Ein PR, der Spalten umbenennt, Typen aendert oder
# Konsensregeln umstellt, muss das unter "Sicherheit" nennen; vor seinem
# Merge auf C1 `docker image rm aequitas-node:vorher` -- dann bricht dieses
# Skript ab, statt ein unvertraegliches Image zu starten.
set -euo pipefail
ENVF=/root/Aequitas/deploy/validator/.env
grep -qx 'ANNAHME_ROLLE=annehmend' "$ENVF" || { echo "ABBRUCH: ANNAHME_ROLLE=annehmend fehlt in $ENVF"; exit 1; }
docker image inspect aequitas-node:vorher >/dev/null 2>&1 || { echo "ABBRUCH: kein aequitas-node:vorher -- nichts zurueckzunehmen"; exit 1; }

echo "Zurueck auf: $(docker image inspect aequitas-node:vorher --format '{{.Id}}')"
docker tag aequitas-node:vorher aequitas-node:local
cd /root/Aequitas/deploy/validator
docker compose up -d --no-build node

for i in $(seq 1 60); do
  S=$(curl -s -m 5 http://127.0.0.1:8080/api/health/combined || true)
  echo "$S" | grep -q '"nimmt_an"' && break
  sleep 5
done
echo "$S" | grep -qE '"nimmt_an": ?true' || { echo "FEHLER: C1 nimmt auch nach der Ruecknahme nicht an"; exit 1; }
echo "C1 nimmt wieder an (voriges Image)."
