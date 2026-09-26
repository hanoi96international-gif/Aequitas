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

GIT_COMMIT="$(git -C /root/Aequitas rev-parse --short HEAD)" docker compose up -d
echo "gestartet; warte auf den NODE_KEY des ersten Starts"

# Beim ersten Start erzeugt der Knoten seinen P2P-Schluessel und druckt ihn
# einmal. Er kommt in die .env (sonst neue Identitaet bei jedem Neustart).
if ! grep -q '^NODE_KEY=' "$ENVF"; then
  for i in $(seq 1 60); do
    K="$(docker logs aequitas-node 2>&1 | grep -A1 'SAVE THIS AS NODE_KEY' | tail -1 | tr -d '[:space:]')"
    if [ -n "$K" ] && [ "${#K}" -gt 40 ]; then
      echo "NODE_KEY=$K" >> "$ENVF"
      echo "NODE_KEY gesichert (nicht ausgegeben); Knoten neu starten, damit er ihn liest"
      docker compose up -d --force-recreate node
      break
    fi
    sleep 2
  done
fi

echo "=== warte auf die API (bis 5 min) ==="
for i in $(seq 1 150); do
  curl -fsS http://127.0.0.1:8080/api/status >/dev/null 2>&1 && break
  sleep 2
done
echo "lokal: $(curl -s http://127.0.0.1:8080/api/status | grep -oE '"height":[0-9]+' || echo 'noch nicht erreichbar')"
echo "Netz:  $(curl -s https://aequitas.digital/api/status | grep -oE '"height":[0-9]+' || echo 'nicht erreichbar')"
echo "=== Knoten-Log (gefiltert) ==="
docker logs --tail 80 aequitas-node 2>&1 | sauber | tail -60
docker ps --format '{{.Names}}\t{{.Status}}'
