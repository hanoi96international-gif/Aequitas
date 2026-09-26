#!/usr/bin/env bash
# Neuen Server vorbereiten, ohne einen Knoten zu starten.
#
# Laeuft ueber .github/workflows/neuer-server-einrichten.yml auf der neuen Box
# (Anmeldung per CONTABO_SSH_KEY, den der Betreiber beim Anbieter hinterlegt
# hat). Installiert Docker und Firewall, holt beide Repos und baut die Images
# von Knoten und Proof-Server. Gestartet wird nichts: die Box soll C1
# uebernehmen, und zwei Knoten mit derselben Identitaet duerfen nie
# gleichzeitig laufen. Gestartet wird erst beim Umzug.
#
# Mehrfach ausfuehrbar.

set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

log() { printf '\n=== %s ===\n' "$*"; }

log "System"
uname -a
. /etc/os-release && echo "$PRETTY_NAME"
nproc
free -h
df -h /

log "Pakete"
apt-get update -qq
apt-get upgrade -y -qq
apt-get install -y -qq git curl openssl ufw ca-certificates
if ! command -v docker >/dev/null; then
  curl -fsSL https://get.docker.com | sh
fi
docker --version
docker compose version

log "Firewall: 22 SSH, 80/443 Caddy, 8080 API, 4001 P2P"
# 22 zuerst, sonst sperrt sich die Anmeldung selbst aus.
for p in 22 80 443 8080 4001; do ufw allow "$p/tcp" >/dev/null; done
ufw --force enable >/dev/null
ufw status | head -20

log "Repos"
if [ -d /root/Aequitas/.git ]; then
  git -C /root/Aequitas fetch -q origin main
  git -C /root/Aequitas checkout -q main
  git -C /root/Aequitas reset -q --hard origin/main
else
  git clone -q https://github.com/hanoi96international-gif/Aequitas.git /root/Aequitas
fi
# Der Proof-Server ist ein privates Repo; ihn richtet ein Workflow im
# Proof-Server-Repo ein.
echo "Aequitas: $(git -C /root/Aequitas rev-parse --short HEAD)"

log "Images bauen (dauert)"
cd /root/Aequitas/deploy/validator
# Compose prueft beim Bauen alle Variablen; das Passwort selbst braucht der
# Bau nicht.
POSTGRES_PASSWORD=nur-fuer-den-bau GIT_COMMIT="$(git -C /root/Aequitas rev-parse --short HEAD)" \
  docker compose build --quiet node
if [ -f /root/aequitas-proof-server/Dockerfile ]; then
  docker build -q -t aequitas-proof-server:latest /root/aequitas-proof-server >/dev/null
else
  echo "Proof-Server-Quelltext fehlt noch (/root/aequitas-proof-server) -- Image nicht gebaut"
fi
docker images --format '{{.Repository}}:{{.Tag}}  {{.Size}}' | grep aequitas

log "Fertig vorbereitet -- kein Knoten gestartet"
df -h /
