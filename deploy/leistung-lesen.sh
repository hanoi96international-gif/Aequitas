#!/usr/bin/env bash
# Nur lesend: Leistungsschalter des laufenden Knotens und Datenbank-Einstellungen.
# Feste Liste, Werte sind Zahlen/Schalter -- keine Schluessel.
LISTE='^(BLOCK_TIME_MS|ENABLE_MULTI_BLOCK_TICK|AEQUITAS_MAX_TXS_PER_BLOCK|AEQUITAS_WAL_ENABLED|AEQUITAS_WAL_PATH|AEQUITAS_RPC_RATE_LIMIT_MAX|AEQUITAS_RPC_RATE_LIMIT_FREI|AEQUITAS_EIGENLAST_BREMSE|AEQUITAS_VORLADEN|AEQUITAS_COMPRESS_BLOCK_PAYLOAD|AEQUITAS_PRODUCE_WHEN_BACKLOG_SHRINKING|AEQUITAS_PEER_LAG_[A-Z_]+|AEQUITAS_INFLIGHT_[A-Z_]+|AEQUITAS_RPC_BATCH_PARALLEL|AEQUITAS_RPC_QUIET_TX|AEQUITAS_TRANSFER_BATCH_[A-Z_]+|AEQUITAS_PARALLEL_[A-Z_]+|AEQUITAS_DB_MAX_[A-Z_]+|AEQUITAS_TX_BATCH_MAX_BYTES|AEQUITAS_CONTENTION_PROFILE|GOMAXPROCS|GOMEMLIMIT|GOGC|ANNAHME_ROLLE|AEQUITAS_LEITUNG[A-Z_]*|AEQUITAS_LEITER_FAEHIG)='
echo "=== $(hostname): $(nproc) Kerne, $(free -g | awk '/Mem:/{print $2}') GB RAM ==="
docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E "$LISTE" | sort
echo "--- Postgres ---"
PG=$(docker ps --format '{{.Names}}' | grep -E '^aequitas-postgres$' | head -1)
for k in max_connections shared_buffers effective_cache_size work_mem synchronous_commit wal_buffers max_wal_size checkpoint_timeout; do
  echo "$k=$(docker exec "$PG" psql -U postgres -tAc "SHOW $k" 2>/dev/null)"
done
echo "--- Knoten ---"
curl -s -m 5 http://127.0.0.1:8080/api/health/combined | python3 -c "
import sys,json; d=json.load(sys.stdin)
for k in ('annahme_tor','wal','db_pool','inflight','block_tick'):
  v=d.get(k)
  if v is not None: print(k, json.dumps(v)[:300])" 2>/dev/null
