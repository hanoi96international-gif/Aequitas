#!/bin/bash
# Proof-Server ausrollen -- aus einem fertigen Image, mit Ruecknahme.
#
# Liegt im Ketten-Repo (Aequitas), nicht mehr im Proof-Server-Repo
# (28.10.2026): das ist privat, und dort gibt es keine Pflicht-Freigabe fuer
# `produktion`. Ausgerollt wird nur noch aus Aequitas
# (proof-server-ausrollen.yml, proof-einzelbetrieb-c1.yml), Umgebung
# produktion mit Freigabe. Abgeleitet aus aequitas-proof-server
# deploy/proof-deploy.sh -- gleiche Sicherung der Umgebung, gleiche
# Schluessellisten, gleiche Modus-Pruefung, gleiche Ruecknahme. Neu:
#
#   IMAGE gesetzt   das Image aus GHCR (vorher angemeldet) ziehen und es
#                   ausrollen. Nur ghcr.io/hanoi96international-gif/
#                   aequitas-proof-server/proof-server:<40-stelliger SHA>.
#   IMAGE leer      das LAUFENDE Image behalten und nur die Umgebung neu
#                   anwenden (festgelegte Schluessellisten, Modus). Gebaut
#                   wird auf der Box nie mehr.
#
# Aufruf:  IMAGE=... ERWARTETER_BIO_MODUS=test bash /root/proof-ausrollen.sh
set -euo pipefail

NETZ=aequitas-net
ENVD=/root/proof-server.env

if [ -n "${IMAGE:-}" ]; then
  [[ "$IMAGE" =~ ^ghcr\.io/hanoi96international-gif/aequitas-proof-server/proof-server:[0-9a-f]{40}$ ]] \
    || { echo "FEHLER: IMAGE muss ghcr.io/hanoi96international-gif/aequitas-proof-server/proof-server:<SHA> sein -- nichts geaendert"; exit 1; }
fi

ALT=$(docker inspect proof-server --format '{{.Config.Image}}' 2>/dev/null || echo "")
[ -n "$ALT" ] || { echo "FEHLER: kein laufender proof-server -- dieses Skript ersetzt, es richtet nicht ein"; exit 1; }
echo "  laufendes Image: $ALT"

# 1. Konfiguration sichern. PATH/NODE_VERSION/YARN_VERSION kommen aus dem
#    Image und wuerden beim Wiedereinsetzen die des NEUEN Images ueberschreiben.
umask 077
[ -f "$ENVD" ] && cp -a "$ENVD" "$ENVD.bak-$(date +%s)"
docker inspect proof-server --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -vE '^(PATH|NODE_VERSION|YARN_VERSION|HOSTNAME|HOME)=' \
  | grep -E '^[A-Z_]+=' > "$ENVD"
chmod 600 "$ENVD"
echo "  gesicherte Variablen (nur Namen): $(cut -d= -f1 "$ENVD" | sort | tr '\n' ' ')"
# 1b. Festgelegte Schluessellisten (schluessel-festlegen.yml) gewinnen gegen
#     den laufenden Stand. Ohne diesen Schritt wuerde die Sicherung oben jede
#     Aenderung an den Listen beim naechsten Deploy wieder ueberschreiben --
#     sie liest ja den ALTEN Container aus.
FEST=/root/proof-server.env.schluessel
if [ -f "$FEST" ]; then
  for k in $(grep -oE '^[A-Z_]+=' "$FEST" | tr -d '='); do
    grep -v "^$k=" "$ENVD" > "$ENVD.tmp" || true
    mv "$ENVD.tmp" "$ENVD"
  done
  cat "$FEST" >> "$ENVD"
  chmod 600 "$ENVD"
  echo "  festgelegte Schluessellisten angewendet: $(cut -d= -f1 "$FEST" | tr '\n' ' ')"
fi

# 1c. Der erwartete Bio-Modus kommt aus dem Workflow (deploy.yml), nicht
#     aus dem alten Container: er ist eine Entscheidung im Repo, die per PR
#     wechselt. Nur test oder real; alles andere bricht ab, bevor etwas
#     geaendert ist.
if [ -n "${ERWARTETER_BIO_MODUS:-}" ]; then
  case "$ERWARTETER_BIO_MODUS" in test|real) ;; *) echo "FEHLER: ERWARTETER_BIO_MODUS=$ERWARTETER_BIO_MODUS (nur test oder real)"; exit 1;; esac
  grep -v '^ERWARTETER_BIO_MODUS=' "$ENVD" > "$ENVD.tmp" || true
  mv "$ENVD.tmp" "$ENVD"
  echo "ERWARTETER_BIO_MODUS=$ERWARTETER_BIO_MODUS" >> "$ENVD"
  chmod 600 "$ENVD"
  echo "  erwarteter Bio-Modus: $ERWARTETER_BIO_MODUS"
fi

# Der Modus, der nach dem Start gelten MUSS: aus der Sicherung (Workflow oder
# alter Container), sonst der Standard real (Sicherheitspruefung #14: auch
# ohne Workflow-Wert wird nach dem Start geprueft).
MODUS_SOLL=$( { grep -E '^ERWARTETER_BIO_MODUS=' "$ENVD" || true; } | tail -1 | cut -d= -f2- | tr -d '[:space:]' | tr 'A-Z' 'a-z')
MODUS_SOLL=${MODUS_SOLL:-real}

for k in DATABASE_URL CHAIN_SERVICE_TOKEN BIO_ATTESTATION_MODE; do
  grep -q "^$k=" "$ENVD" || { echo "FEHLER: $k fehlt in der Sicherung -- abgebrochen, nichts geaendert"; exit 1; }
done

# 2. Image: aus GHCR ziehen, oder das laufende behalten.
if [ -n "${IMAGE:-}" ]; then
  ok=0
  for pause in 5 15 30; do
    if docker pull -q "$IMAGE" >/dev/null; then ok=1; break; fi
    echo "  Ziehen gescheitert -- neuer Versuch in ${pause}s"; sleep "$pause"
  done
  [ "$ok" = 1 ] || { echo "FEHLER: $IMAGE nicht ziehbar -- nichts geaendert"; exit 1; }
  NEU="$IMAGE"
  echo "  gezogen: $NEU"
else
  NEU="$ALT"
  echo "  kein IMAGE: laufendes Image bleibt, nur die Umgebung wird neu angewendet"
fi

# 3. Ersetzen und pruefen.
# Log-Rotation am Container selbst (Server-Haertung, Betreiber-Punkt 13):
# die Vorgabe in daemon.json gilt erst nach einem Docker-Neustart, der alle
# Container anhielte. So rotiert proof-server ab dem naechsten Deploy.
LOGS="--log-opt max-size=50m --log-opt max-file=5"
docker rm -f proof-server >/dev/null 2>&1 || true
docker run -d --name proof-server --restart unless-stopped \
  --network "$NETZ" --env-file "$ENVD" $LOGS "$NEU" >/dev/null

pruefe() {
  for _ in $(seq 1 20); do
    H=$(docker exec proof-server sh -c 'wget -qO- --timeout=3 http://127.0.0.1:3000/health' 2>/dev/null || true)
    if echo "$H" | grep -q '"status"'; then
      # Laeuft er im Modus, den der Workflow verlangt? Sonst gilt der Deploy
      # als gescheitert (Ruecknahme), statt still alle Registrierungen
      # abzuweisen.
      if [ "${PRUEFE_MODUS:-1}" = 1 ] \
         && ! echo "$H" | grep -qE "\"erwarteter_modus\": ?\"$MODUS_SOLL\""; then
        echo "  ✗ /health meldet nicht erwarteter_modus=$MODUS_SOLL"
        return 1
      fi
      return 0
    fi
    sleep 3
  done
  return 1
}

if pruefe; then
  echo "  ✓ /health antwortet -- $NEU ist aktiv"
  docker exec proof-server sh -c 'wget -qO- --timeout=3 http://127.0.0.1:3000/health' | head -c 400; echo
else
  echo "  ✗ /health antwortet nicht -- nehme $ALT zurueck"
  docker rm -f proof-server >/dev/null 2>&1 || true
  docker run -d --name proof-server --restart unless-stopped \
    --network "$NETZ" --env-file "$ENVD" $LOGS "$ALT" >/dev/null
  PRUEFE_MODUS=0 pruefe && echo "  zurueckgenommen, alter Stand laeuft wieder" || echo "  ACHTUNG: auch der alte Stand antwortet nicht"
  exit 1
fi
