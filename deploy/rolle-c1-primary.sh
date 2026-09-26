#!/usr/bin/env bash
# C1 (netcup) ist der Primary: er allein nimmt Ueberweisungen an, und
# IS_PRIMARY_NODE=true laesst RESET_DB_STATE hier nie greifen.
# Laeuft erst, nachdem C2 nimmt_an=false gemeldet hat.
set -euo pipefail
ENVF=/root/Aequitas/deploy/validator/.env
setze() { sed -i "/^$1=/d" "$ENVF"; echo "$1=$2" >> "$ENVF"; }
cp -a "$ENVF" "$ENVF.bak-rolle-$(date +%s)"
setze ANNAHME_ROLLE annehmend
setze IS_PRIMARY_NODE true
chmod 600 "$ENVF"
cd /root/Aequitas/deploy/validator
GIT_COMMIT="$(git -C /root/Aequitas rev-parse --short HEAD)" docker compose up -d --force-recreate node
echo "=== im Prozess ==="
docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E '^(ANNAHME_ROLLE|IS_PRIMARY_NODE|PRIMARY_NODE_URLS)=' | sort
for i in $(seq 1 60); do
  S=$(curl -s -m 5 http://127.0.0.1:8080/api/health/combined || true)
  echo "$S" | grep -q '"nimmt_an"' && break
  sleep 5
done
echo "$S" | grep -qE '"nimmt_an": ?true' || { echo "FEHLER: C1 meldet nicht nimmt_an=true"; exit 1; }
curl -s -m 5 http://127.0.0.1:8080/api/status | grep -oE '"(height|is_primary)":[^,]+' 
echo "C1 ist Primary und nimmt an."
