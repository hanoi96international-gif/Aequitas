#!/usr/bin/env bash
# Leistungsschalter auf C1 (netcup, Primary) wie auf C2 setzen.
#
# Gelesen am 26.09.2026 mit deploy/leistung-lesen.sh: C1 hat WAL, aber
# keinen der Schalter, die C2 seit August traegt (Multi-Block-Tick, 7000 Tx
# je Block, komprimierte Bloecke, DB-Pool 100 statt 20, ...). C1 ist der
# annehmende Knoten -- genau dort zaehlen sie.
#
# Nur Zahlen und Schalter, keine Schluessel. Idempotent. Die Sicherung
# .env.bak.<zeit> ist der vollstaendige Rueckweg:
#   cp .env.bak.<zeit> .env && docker compose up -d --no-deps node
# Laeuft ueber .github/workflows/perf-c1.yml.
set -euo pipefail
cd /root/Aequitas/deploy/validator
ENVF=.env
grep -qx 'ANNAHME_ROLLE=annehmend' "$ENVF" || { echo "ABBRUCH: ANNAHME_ROLLE=annehmend fehlt in $ENVF"; exit 1; }
cp -a "$ENVF" "$ENVF.bak.$(date +%s)"

setze() {
  if grep -q "^$1=" "$ENVF"; then sed -i "s|^$1=.*|$1=$2|" "$ENVF"; else printf '%s=%s\n' "$1" "$2" >> "$ENVF"; fi
}
setze ENABLE_MULTI_BLOCK_TICK 1
setze AEQUITAS_MAX_TXS_PER_BLOCK 7000
setze AEQUITAS_COMPRESS_BLOCK_PAYLOAD 1
setze AEQUITAS_DB_MAX_CONNS 100
setze AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING 1
setze AEQUITAS_RPC_RATE_LIMIT_MAX 1000
setze AEQUITAS_RPC_QUIET_TX 1
setze AEQUITAS_EIGENLAST_BREMSE 0
setze AEQUITAS_PEER_LAG_BODEN 1500
setze AEQUITAS_PEER_LAG_SLACK 5
# 15 GB RAM auf C1 (C2: 11 GB, 5GiB). Postgres braucht ~2 GB, Rest bleibt frei.
setze GOMEMLIMIT 8GiB
# Lastgenerator auf C2 (eigene Maschine) von der Ratenbegrenzung ausnehmen
# (rpc_frei.go). Gemessen 26.09.: ohne das 99 % -32005 bei 1.051 Paaren.
# Nur diese eine Adresse; die Inflight-Grenze gilt weiter.
setze AEQUITAS_RPC_RATE_LIMIT_FREI 194.163.188.71

# Kein Neubau: dasselbe Image, nur neue Umgebung.
docker compose up -d --no-deps node

LISTE='^(ENABLE_MULTI_BLOCK_TICK|AEQUITAS_MAX_TXS_PER_BLOCK|AEQUITAS_COMPRESS_BLOCK_PAYLOAD|AEQUITAS_DB_MAX_CONNS|AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING|AEQUITAS_RPC_RATE_LIMIT_MAX|AEQUITAS_RPC_QUIET_TX|AEQUITAS_EIGENLAST_BREMSE|AEQUITAS_PEER_LAG_BODEN|AEQUITAS_PEER_LAG_SLACK|GOMEMLIMIT|AEQUITAS_RPC_RATE_LIMIT_FREI|AEQUITAS_WAL_ENABLED|ANNAHME_ROLLE|IS_PRIMARY_NODE)='
echo "--- im Container ---"
docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E "$LISTE" | sort
n=$(docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -cE "$LISTE")
[ "$n" -ge 15 ] || { echo "FEHLER: nur $n von 15 Schaltern im Container"; exit 1; }

for i in $(seq 1 72); do
  S=$(curl -s -m 5 http://127.0.0.1:8080/api/health/combined || true)
  echo "$S" | grep -q '"nimmt_an"' && break
  sleep 5
done
echo "$S" | grep -qE '"nimmt_an": ?true' || { echo "FEHLER: C1 nimmt nach dem Neustart nicht an"; exit 1; }
echo "C1 nimmt an."
echo "$S" | python3 -c "
import sys,json; d=json.load(sys.stdin)
print('db_pool max_open', (d.get('db_pool') or {}).get('max_open'))" 2>/dev/null || true
