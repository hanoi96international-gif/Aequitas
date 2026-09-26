#!/usr/bin/env bash
# Nur lesend: Rolle, Dienste und Quorum auf EINER Box. Das Actions-Log ist
# oeffentlich -- ausgegeben werden nur Namen aus festen Listen, deren Werte
# Adressen, Zahlen oder Schalter sind. Schluessel, Tokens, Datenbank-URLs
# werden nie gelesen.
set -uo pipefail
KNOTEN_LISTE='^(ANNAHME_ROLLE|IS_PRIMARY_NODE|AEQUITAS_LEITUNG[A-Z_]*|AEQUITAS_LEITER_FAEHIG|PRIMARY_NODE_URLS?|BOOTSTRAP_SIGNER|SELF_URL|NODE_OPERATOR_WALLET|AUTHORIZED_VALIDATORS|VALIDATOR_LABELS|PROOF_SERVER_URLS?|RESYNC_FROM_SNAPSHOT|AUTO_HEAL_ON_DIVERGENCE|BLOCK_TIME_MS|ENABLE_MULTI_BLOCK_TICK|REQUIRE_PROVE_PROVENANCE|AEQUITAS_WACHE_COORDINATOR_URL)='
DIENST_LISTE='^(QUORUM_SIZE|VALIDATOR_URLS|SERVICE_MODE|CHAIN_BASE_URL|PROOF_SERVER_URLS?|BIO_ATTESTATION_QUORUM|BIO_ATTESTATION_[A-Z_]*PUBKEY[A-Z_]*|COORDINATOR_URLS?|NODE_ENV|PORT|LEBENDIGKEIT_VERBINDLICH|REAL_DATA_ENABLED|GATE_[A-Z_]*|ALLOWED_[A-Z_]*)='

echo "=== Box: $(hostname) $(curl -fsS4 -m 5 https://api.ipify.org 2>/dev/null) ==="
echo "--- Container ---"
docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}\t{{.Ports}}'
echo "--- Knoten: Rolle und Netz (feste Liste) ---"
docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E "$KNOTEN_LISTE" | sort || true
echo "(nicht gezeigt = nicht gesetzt)"
for c in $(docker ps -a --format '{{.Names}}' | grep -vE '^(aequitas-node|aequitas-postgres|postgres|aequitas-caddy|caddy)$'); do
  echo "--- $c: Einstellungen (feste Liste) und Namen aller anderen ---"
  docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E "$DIENST_LISTE" | sort
  echo "  weitere Namen: $(docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -vE "$DIENST_LISTE" | cut -d= -f1 | grep -vE '^(PATH|HOME|HOSTNAME|LANG|GPG_KEY|PYTHON_[A-Z_]*|NODE_VERSION|YARN_VERSION)$' | tr '\n' ' ')"
  echo "  Netze: $(docker inspect "$c" --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}')"
done
echo "--- Caddy ---"
for f in /root/caddy/Caddyfile /root/Caddyfile /etc/caddy/Caddyfile; do [ -f "$f" ] && { echo "# $f"; grep -vE '^\s*#' "$f" | grep -v '^\s*$'; }; done
docker exec aequitas-caddy cat /etc/caddy/Caddyfile 2>/dev/null | grep -vE '^\s*#' | grep -v '^\s*$' | head -80 || true
echo "--- Knoten-API (oeffentliche Felder) ---"
curl -s -m 5 http://127.0.0.1:8080/api/status | head -c 1500; echo
echo "--- /api/wache ---"
curl -s -m 15 http://127.0.0.1:8080/api/wache | head -c 3000; echo
echo "--- /api/leitung ---"
curl -s -m 5 http://127.0.0.1:8080/api/leitung | head -c 1000; echo
echo "--- Knoten-Log: Annahme/Bloecke/Peers (gefiltert) ---"
docker logs --since 20m aequitas-node 2>&1 | grep -vE '[A-Za-z0-9+/=]{60,}|PRIVATE|SECRET|TOKEN|PASSWORD' \
  | grep -E 'ANNAHME|nur_lesend|Block #|produc|PEERS|binding|VALIDATOR|ERROR|WARN' | tail -30
echo "--- Dienste von innen ---"
for u in aequitas-proof-server:3000/health proof-server:3000/health aequitas-coordinator:8200/health aequitas-coordinator:8200/inventory aequitas-matching:8098/health; do
  r=$(docker exec aequitas-node wget -qO- -T 5 "http://$u" 2>/dev/null | head -c 1500)
  [ -n "$r" ] && echo "$u -> $r"
done
echo "--- Maschine ---"
nproc; free -m | head -2; df -h / | tail -1; uptime
