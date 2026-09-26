#!/usr/bin/env bash
# Neuen Server als Aequitas-Validator MIT eigenem Proof-Server einrichten.
#
# Auf dem frischen Server (Ubuntu 22.04/24.04, als root):
#
#   curl -fsSL https://raw.githubusercontent.com/hanoi96international-gif/Aequitas/claude/aequitas-weitermachen-i6yayi/deploy/neuer-server.sh \
#     | bash -s -- 0xDEIN_REGISTRIERTES_WALLET
#
# Das Wallet muss ein in der App registrierter Mensch sein
# (docs/VALIDATOR_EINRICHTEN.md: ein Mensch = ein Validator).
#
# Was passiert:
#   1. System aktualisieren, Docker + git + ufw installieren.
#   2. Firewall: nur 22 (SSH), 8080 (API) und 4001 (P2P) offen.
#   3. Validator aus deploy/validator (Stand main) bauen und starten.
#      Passwoerter und Dienst-Token werden hier erzeugt und bleiben auf der
#      Box (/root/Aequitas/deploy/validator/.env, nur root lesbar).
#   4. Eigene Datenbank fuer den Proof-Server in derselben Postgres-Instanz,
#      Proof-Server bauen und im internen Docker-Netz starten -- nicht von
#      aussen erreichbar, der Knoten reicht /api/prove* an ihn weiter.
#
# Mehrfach ausfuehrbar: vorhandene .env-Dateien werden nicht ueberschrieben.

set -euo pipefail

WALLET="${1:-}"
IP="$(curl -fsS4 https://api.ipify.org || hostname -I | awk '{print $1}')"
REPO=/root/Aequitas
PROOF_REPO=/root/aequitas-proof-server
VAL_ENV="$REPO/deploy/validator/.env"
PROOF_ENV=/root/proof-server.env

log()  { printf '\n[einrichten] %s\n' "$*"; }
fail() { printf '\n[einrichten] FEHLER: %s\n' "$*" >&2; exit 1; }
zufall() { openssl rand -hex 24; }

[ "$(id -u)" = 0 ] || fail "als root ausfuehren"
[[ "$WALLET" =~ ^0x[0-9a-fA-F]{40}$ ]] || fail "Wallet-Adresse fehlt oder ist ungueltig. Aufruf: bash -s -- 0x..."

log "1/5 System und Docker"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get upgrade -y -qq
apt-get install -y -qq git curl openssl ufw ca-certificates
if ! command -v docker >/dev/null; then
  curl -fsSL https://get.docker.com | sh
fi
docker compose version >/dev/null || fail "Docker Compose fehlt"

log "2/5 Firewall (22, 8080, 4001)"
ufw allow 22/tcp >/dev/null
ufw allow 8080/tcp >/dev/null
ufw allow 4001/tcp >/dev/null
ufw --force enable >/dev/null

log "3/5 Validator"
if [ -d "$REPO/.git" ]; then
  git -C "$REPO" fetch -q origin main && git -C "$REPO" checkout -q main && git -C "$REPO" reset -q --hard origin/main
else
  git clone -q https://github.com/hanoi96international-gif/Aequitas.git "$REPO"
fi
if [ ! -f "$VAL_ENV" ]; then
  umask 077
  cat > "$VAL_ENV" <<EOF
POSTGRES_PASSWORD=$(zufall)
SELF_URL=http://$IP:8080
NODE_OPERATOR_WALLET=$WALLET
CHAIN_SERVICE_TOKEN=$(zufall)
PROOF_SERVER_URLS=http://aequitas-proof-server:3000
# Werden beim ersten Start erzeugt und EINMAL ins Log gedruckt ("SAVE THIS AS").
# Danach hier eintragen und neu starten, sonst neue Identitaet bei jedem Neustart.
RELAYER_PRIVATE_KEY=
NODE_KEY=
EOF
  umask 022
fi
set -a; . "$VAL_ENV"; set +a
cd "$REPO/deploy/validator"
GIT_COMMIT="$(git -C "$REPO" rev-parse --short HEAD)" docker compose up -d --build

log "4/5 Proof-Server"
for i in $(seq 1 60); do
  docker exec aequitas-postgres pg_isready -U postgres -d aequitas >/dev/null 2>&1 && break
  sleep 2
done
docker exec aequitas-postgres psql -U postgres -tAc "SELECT 1 FROM pg_database WHERE datname='aequitas_proof'" | grep -q 1 \
  || docker exec aequitas-postgres psql -U postgres -c "CREATE DATABASE aequitas_proof" >/dev/null
if [ ! -f "$PROOF_ENV" ]; then
  umask 077
  cat > "$PROOF_ENV" <<EOF
DATABASE_URL=postgres://postgres:$POSTGRES_PASSWORD@aequitas-postgres:5432/aequitas_proof
CHAIN_SERVICE_TOKEN=$CHAIN_SERVICE_TOKEN
CHAIN_BASE_URL=http://aequitas-node:8080
NODE_ENV=production
PORT=3000
DB_SSL_REJECT_UNAUTHORIZED=false
EOF
  umask 022
fi
if [ -d "$PROOF_REPO/.git" ]; then
  git -C "$PROOF_REPO" fetch -q origin main && git -C "$PROOF_REPO" reset -q --hard origin/main
else
  git clone -q https://github.com/hanoi96international-gif/aequitas-proof-server.git "$PROOF_REPO"
fi
docker build -q -t aequitas-proof-server:latest "$PROOF_REPO" >/dev/null
docker rm -f aequitas-proof-server >/dev/null 2>&1 || true
# Kein Port nach aussen: nur im Netz aequitas-net, der Knoten erreicht ihn
# unter aequitas-proof-server:3000.
docker run -d --name aequitas-proof-server --restart unless-stopped \
  --network aequitas-net --env-file "$PROOF_ENV" \
  --log-opt max-size=10m --log-opt max-file=3 \
  aequitas-proof-server:latest >/dev/null

log "5/5 Pruefen (bis zu 3 Minuten)"
for i in $(seq 1 90); do
  curl -fsS http://127.0.0.1:8080/api/status >/dev/null 2>&1 && break
  sleep 2
done
echo "Knoten lokal:  $(curl -s http://127.0.0.1:8080/api/status | grep -oE '"height":[0-9]+' || echo 'noch nicht erreichbar')"
echo "Netz (Seeds):  $(curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+' || echo 'nicht erreichbar')"
echo "Proof-Server:  $(docker exec aequitas-node wget -qO- http://aequitas-proof-server:3000/health 2>/dev/null | head -c 120 || echo 'noch nicht bereit')"
echo
echo "Schluessel aus dem Log (falls schon erzeugt):"
docker logs aequitas-node 2>&1 | grep -A2 "SAVE THIS AS" || echo "  noch keine -- spaeter: docker logs aequitas-node 2>&1 | grep -A2 'SAVE THIS AS'"
echo
echo "FERTIG. Naechste Schritte:"
echo "  1. RELAYER_PRIVATE_KEY und NODE_KEY aus dem Log in $VAL_ENV eintragen,"
echo "     dann: cd $REPO/deploy/validator && docker compose up -d"
echo "  2. Aufholen beobachten: docker compose -f $REPO/deploy/validator/docker-compose.yml logs -f node"
echo "  3. Selbstpruefung: curl -s http://$IP:8080/api/wache"
echo "  4. Root-Passwort aendern: passwd"
