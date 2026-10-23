#!/usr/bin/env bash
# Neuen C1 (netcup, Primary) auf origin/main bringen.
# Laeuft ueber .github/workflows/deploy-c1.yml -- danach, und nur
# wenn das hier gruen ist, zieht C2 nach.
#
# Die Rolle steht in deploy/validator/.env (ANNAHME_ROLLE=annehmend,
# IS_PRIMARY_NODE=true, PROOF_SERVER_URLS) und ist von git nicht erfasst --
# ein reset aendert sie nicht. Geprueft wird sie trotzdem, vorher und nachher.
set -euo pipefail
ENVF=/root/Aequitas/deploy/validator/.env
grep -qx 'ANNAHME_ROLLE=annehmend' "$ENVF" || { echo "ABBRUCH: ANNAHME_ROLLE=annehmend fehlt in $ENVF"; exit 1; }

# Ruecknahme (deploy/rueckrollen-c1.sh): das Image, das JETZT laeuft, als
# aequitas-node:vorher merken -- vom laufenden Container, nicht vom Tag
# :local (der kann nach einem gescheiterten Lauf schon neuer sein). Laeuft
# kein Knoten, gibt es nichts zurueckzunehmen, und ein altes :vorher wird
# entfernt, damit nie ein noch aelteres Image zurueckkommt.
if LAUFEND=$(docker inspect aequitas-node --format '{{.Image}}' 2>/dev/null) && [ -n "$LAUFEND" ]; then
  docker tag "$LAUFEND" aequitas-node:vorher
  echo "Ruecknahme moeglich auf: $LAUFEND"
else
  docker image rm aequitas-node:vorher >/dev/null 2>&1 || true
  echo "Kein laufender Knoten -- keine Ruecknahme moeglich"
fi

cd /root/Aequitas
git fetch -q origin main
git reset -q --hard origin/main
echo "Stand: $(git rev-parse --short=7 HEAD)"
df -h / | tail -1
cd deploy/validator
# Erst bauen, waehrend der alte Knoten weiterlaeuft; der Neustart danach
# dauert nur Sekunden.
GIT_COMMIT="$(git -C /root/Aequitas rev-parse --short=7 HEAD)" docker compose build -q node
GIT_COMMIT="$(git -C /root/Aequitas rev-parse --short=7 HEAD)" docker compose up -d node
docker image prune -f >/dev/null 2>&1 || true
docker builder prune -af --max-used-space 10GB >/dev/null 2>&1 || true

docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E '^(ANNAHME_ROLLE|IS_PRIMARY_NODE|PROOF_SERVER_URLS|PRIMARY_NODE_URLS)=' | sort
for i in $(seq 1 60); do
  S=$(curl -s -m 5 http://127.0.0.1:8080/api/health/combined || true)
  echo "$S" | grep -q '"nimmt_an"' && break
  sleep 5
done
echo "$S" | grep -qE '"nimmt_an": ?true' || { echo "FEHLER: C1 nimmt nach dem Deploy nicht an"; exit 1; }
echo "C1 nimmt an."
