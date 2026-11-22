#!/usr/bin/env bash
# Probe von deploy/aufraeumen-c1.sh mit echtem Docker (root, Docker-Daemon).
# Weg muessen: ein anonymes, unbenutztes Volume; ein unbenutztes Image.
# Bleiben muessen: das benannte Datenvolume, das anonyme Volume eines
# ANGEHALTENEN Containers, das Image eines Containers, das geschuetzte
# aequitas-node:vorher. pruefen loescht nichts.
set -euo pipefail
cd "$(dirname "$0")/.."
BILD="${BILD:-alpine:3.20}"
fehl() { echo "FEHLER: $*"; exit 1; }
weg() {
  docker rm -f aequitas-node aufr-gestoppt >/dev/null 2>&1 || true
  docker volume rm aufr_daten >/dev/null 2>&1 || true
  [ -n "${ANON_FREI:-}" ] && docker volume rm "$ANON_FREI" >/dev/null 2>&1 || true
  docker rmi aequitas-node:vorher aufr-unbenutzt:1 >/dev/null 2>&1 || true
}
trap weg EXIT
docker image inspect "$BILD" >/dev/null 2>&1 || docker pull -q "$BILD" >/dev/null
docker build -q -t aequitas-node:vorher - >/dev/null <<EOF
FROM $BILD
RUN echo vorher > /v
EOF
docker build -q -t aufr-unbenutzt:1 - >/dev/null <<EOF
FROM $BILD
RUN echo anders > /x
EOF
docker volume create aufr_daten >/dev/null
ANON_FREI="$(docker volume create)"
docker run -d --name aequitas-node -v aufr_daten:/d "$BILD" sleep 100000 >/dev/null
docker run -d --name aufr-gestoppt -v /anon "$BILD" sleep 100000 >/dev/null
docker stop -t 1 aufr-gestoppt >/dev/null
ANON_GESTOPPT="$(docker inspect aufr-gestoppt --format '{{range .Mounts}}{{.Name}}{{end}}')"

MODUS=pruefen bash deploy/aufraeumen-c1.sh >/dev/null 2>&1 || true
docker volume inspect "$ANON_FREI" >/dev/null 2>&1 || fehl "pruefen hat geloescht"
docker image inspect aufr-unbenutzt:1 >/dev/null 2>&1 || fehl "pruefen hat ein Image geloescht"

# Laeuft der Knoten nicht, wird nichts geloescht.
docker stop -t 1 aequitas-node >/dev/null
MODUS=loeschen bash deploy/aufraeumen-c1.sh >/dev/null 2>&1 && fehl "geloescht, obwohl der Knoten nicht laeuft"
docker volume inspect "$ANON_FREI" >/dev/null 2>&1 || fehl "ohne laufenden Knoten geloescht"
docker start aequitas-node >/dev/null

MODUS=loeschen bash deploy/aufraeumen-c1.sh >/dev/null 2>&1 || true   # der Knoten-Check per curl schlaegt hier fehl (kein :8080)
docker volume inspect "$ANON_FREI" >/dev/null 2>&1 && fehl "unbenutztes anonymes Volume nicht geloescht"
docker image inspect aufr-unbenutzt:1 >/dev/null 2>&1 && fehl "unbenutztes Image nicht geloescht"
docker volume inspect aufr_daten >/dev/null 2>&1 || fehl "benanntes Datenvolume geloescht"
# (Docker selbst verweigert das Loeschen benutzter Volumes -- zweite Sperre.)
docker volume inspect "$ANON_GESTOPPT" >/dev/null 2>&1 || fehl "Volume eines angehaltenen Containers geloescht"
docker image inspect aequitas-node:vorher >/dev/null 2>&1 || fehl "geschuetztes aequitas-node:vorher geloescht"
docker image inspect "$BILD" >/dev/null 2>&1 || fehl "Image eines Containers geloescht"
echo "ok: Aufraeumen geprobt (nur Unbenutztes weg, Daten, Ruecknahme und angehaltene Container bleiben)"
