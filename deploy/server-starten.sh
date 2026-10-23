#!/usr/bin/env bash
# Validator auf der neuen Box starten (ersetzt C1, eigene frische Identitaet).
#
# Laeuft ueber .github/workflows/neuer-server-starten.yml. Das Log dieses
# Laufs ist OEFFENTLICH (das Repo ist oeffentlich): Schluessel werden hier
# auf der Box erzeugt, nur in die .env (600 root) geschrieben und NIE
# ausgegeben. Knoten-Logs gehen nur gefiltert nach draussen.
#
# Mehrfach ausfuehrbar: eine vorhandene .env bleibt, wie sie ist.

set -euo pipefail
WALLET="${1:?Betreiber-Wallet fehlt}"
ENVF=/root/Aequitas/deploy/validator/.env
IP="$(curl -fsS4 https://api.ipify.org)"

# Knoten-Log ohne alles, was nach Schluessel aussieht.
sauber() { grep -vE '[A-Za-z0-9+/=]{60,}|PRIVATE|SECRET|TOKEN|PASSWORD' || true; }

cd /root/Aequitas/deploy/validator
if [ ! -f "$ENVF" ]; then
  umask 077
  {
    echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)"
    echo "SELF_URL=http://$IP:8080"
    echo "NODE_OPERATOR_WALLET=$WALLET"
    echo "CHAIN_SERVICE_TOKEN=$(openssl rand -hex 24)"
    echo "RELAYER_PRIVATE_KEY=$(openssl rand -hex 32)"
  } > "$ENVF"
  umask 022
  echo ".env angelegt (Schluessel auf der Box erzeugt, nicht ausgegeben)"
else
  echo ".env vorhanden -- unveraendert"
fi
chmod 600 "$ENVF"

# Quelle fuer den Snapshot des frischen Knotens (wie auf der Website fuer
# neue Validatoren beschrieben): C2, dessen Blockadresse die Signatur des
# Snapshots beglaubigt. Ohne PRIMARY_NODE_URLS versucht ein leerer Knoten den
# Replay ab Genesis und steht bei Hoehe 0 (so am 26.09.2026).
#
# Seit dem 03.10.2026 ist C2 abgeschaltet und seine IP neu vergeben. Eine
# Quelle, die ein Fremder beantworten kann, waere ein vertrauenswuerdiger
# Seed (FromSync). Darum gibt es hier keine Vorgabe mehr: wer einen neuen
# Server aufsetzt, traegt PRIMARY_NODE_URLS und BOOTSTRAP_SIGNER (Quelle
# und deren Blockadresse) vorher selbst in die .env ein -- sonst Abbruch.
setze() { grep -q "^$1=" "$ENVF" || echo "$1=$2" >> "$ENVF"; }
for k in PRIMARY_NODE_URLS BOOTSTRAP_SIGNER; do
  grep -q "^$k=." "$ENVF" || { echo "ABBRUCH: $k fehlt in $ENVF -- Snapshot-Quelle und Signierer zuerst festlegen (C2 ist abgeschaltet)"; exit 1; }
done
# Bindung Signierschluessel 0x3066639a...42dc an die Menschen-Wallet
# 0x0be8...d016, auf /node-binding erzeugt (26.09.2026, geprueft: stellt
# genau diese Wallet wieder her). Nicht geheim. Der Knoten schickt sie den
# anderen Knoten bei der Anmeldung mit, damit sie ihn als Validator annehmen.
setze NODE_OPERATOR_BINDING_SIGNATURE "0x4ca84728f6d471ab2d499129a540a2e4ca6521e3e95a09bb0d0cc810699553e756dd885edaad3027c9a91b8438abd17c1f08ca9db520ebaaf86e2f41f5f91cf21b"

# Feste Schluessel VOR dem Start, direkt in die .env -- nie ins Log
# (schluessel-eintragen.sh).
GIT_COMMIT="$(git -C /root/Aequitas rev-parse --short HEAD)" docker compose build node
bash ./schluessel-eintragen.sh "$ENVF"
GIT_COMMIT="$(git -C /root/Aequitas rev-parse --short HEAD)" docker compose up -d --force-recreate node
echo "gestartet"

echo "=== warte auf die API (bis 5 min) ==="
for i in $(seq 1 150); do
  curl -fsS http://127.0.0.1:8080/api/status >/dev/null 2>&1 && break
  sleep 2
done
echo "lokal: $(curl -s http://127.0.0.1:8080/api/status | grep -oE '"height":[0-9]+' || echo 'noch nicht erreichbar')"
echo "Netz:  $(curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+' || echo 'nicht erreichbar')"
echo "=== Snapshot-Import beobachten (bis 10 min) ==="
for i in $(seq 1 60); do
  H="$(curl -s http://127.0.0.1:8080/api/status | grep -oE '"height":[0-9]+' | grep -oE '[0-9]+' || echo 0)"
  echo "$(date +%H:%M:%S) Hoehe lokal: $H"
  [ "${H:-0}" -gt 1000 ] && break
  sleep 10
done
echo "Netz:  $(curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+' || echo 'nicht erreichbar')"
echo "=== Knoten-Log (gefiltert) ==="
docker logs aequitas-node 2>&1 | sauber | grep -E 'BOOTSTRAP|RESYNC|HTTP-SYNC|NODE\]|binding|registered|Block #|ERROR' | tail -40
docker ps --format '{{.Names}}\t{{.Status}}'
