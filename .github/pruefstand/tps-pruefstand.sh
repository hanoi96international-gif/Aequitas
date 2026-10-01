# TPS-Pruefstand auf der Box: ein ZWEITER, vollstaendig abgeschotteter Knoten
# aus dem uebergebenen Quellstand, mit eigener Postgres, eigenem WAL und
# Testkonten -- dann Last dagegen, Zahlen ausgeben, alles wieder abraeumen.
#
# Warum auf der Box: Durchsatz haengt an der Platte (fdatasync des WAL) und an
# den Kernen. Ein Sandkasten mit anderer Platte misst etwas anderes.
#
# Was NICHT beruehrt wird: der laufende Knoten aequitas-node, seine Datenbank,
# sein WAL, seine Ports. Der Pruefstand
#   - laeuft in eigenen Containern (pruefstand-*) in einem eigenen Netz,
#   - hat eine eigene, wegwerfbare Postgres (kein Volume, das bleibt),
#   - ist nur ueber 127.0.0.1:18080 erreichbar (P2P nicht veroeffentlicht),
#   - kennt keine Seeds (PRIMARY_NODE_URLS=keine) -- er spricht mit niemandem,
#   - benutzt Wegwerf-Schluessel und Wegwerf-Konten, die mit ihm verschwinden.
# Er kostet waehrend des Laufs CPU und Platte der Box; die Laufzeit ist kurz.
#
# Eingaben (Umgebung): QUELLE (Verzeichnis mit dem entpackten Quellstand),
# NAME (Bezeichnung des Laufs), DAUER (z. B. 40s), KONTEN (z. B. 1000),
# BUENDEL (Ueberweisungen je RPC-Buendel).
set -euo pipefail
: "${QUELLE:?}" "${NAME:?}"
DAUER="${DAUER:-40s}"; KONTEN="${KONTEN:-1000}"; BUENDEL="${BUENDEL:-20}"
NETZ=pruefstand-net; PG=pruefstand-pg; KN=pruefstand-node; PORT=18080

ENVDATEI=""
aufraeumen() {
  [ -n "$ENVDATEI" ] && rm -f "$ENVDATEI"
  docker rm -f "$KN" "$PG" >/dev/null 2>&1 || true
  docker volume rm -f pruefstand-wal >/dev/null 2>&1 || true
  docker network rm "$NETZ" >/dev/null 2>&1 || true
}
trap aufraeumen EXIT
aufraeumen

echo "== Bauen ($NAME)"
docker build -q -t "aequitas-pruefstand:$NAME" "$QUELLE" >/dev/null
docker network create "$NETZ" >/dev/null

echo "== Postgres (wegwerfbar, dieselben Einstellungen wie deploy/validator)"
docker run -d --name "$PG" --network "$NETZ" -e POSTGRES_PASSWORD=pruefstand -e POSTGRES_DB=aequitas \
  postgres:16-alpine postgres -c max_connections=250 -c shared_buffers=1GB -c synchronous_commit=off \
  -c max_wal_size=8GB -c checkpoint_timeout=15min -c wal_compression=on >/dev/null
for i in $(seq 1 30); do docker exec "$PG" pg_isready -U postgres -d aequitas >/dev/null 2>&1 && break; sleep 1; done

# Dieselben Leistungs-Einstellungen wie der laufende Knoten -- nur Namen aus
# einer festen Liste, keine Schluessel, keine Adressen.
TUNING="$(docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
  | grep -E '^(ENABLE_MULTI_BLOCK_TICK|BLOCK_TIME_MS|GOMEMLIMIT|GOGC|AEQUITAS_WAL_(MAX|FLUSH)[A-Z_]*|AEQUITAS_INFLIGHT[A-Z_]*|AEQUITAS_BLOCK[A-Z_]*|AEQUITAS_RUECKSTAU[A-Z_]*)=' || true)"
echo "Einstellungen wie der laufende Knoten:"; printf '%s\n' "$TUNING" | sed 's/^/  /'
ENVDATEI="$(mktemp)"; chmod 600 "$ENVDATEI"
{
  printf '%s\n' "$TUNING"
  echo "DATABASE_URL=postgres://postgres:pruefstand@$PG:5432/aequitas?sslmode=disable"
  echo "RELAYER_PRIVATE_KEY=0x$(openssl rand -hex 32)"
  echo "AEQUITAS_WAL_ENABLED=1"
  echo "AEQUITAS_WAL_PATH=/data/wal/aequitas_transfers.wal"
  echo "PRIMARY_NODE_URLS=keine"
  echo "IS_PRIMARY_NODE=true"
  echo "SELF_URL=http://127.0.0.1:$PORT"
  echo "AUTO_HEAL_ON_DIVERGENCE=false"
  # Nur dieser Pruefstand: der Generator laeuft von EINER Adresse aus, die
  # Begrenzung je Adresse wuerde sonst den Generator messen, nicht den Knoten.
  echo "AEQUITAS_RPC_RATE_LIMIT_MAX=1000000"
} > "$ENVDATEI"

starte() {
  docker rm -f "$KN" >/dev/null 2>&1 || true
  docker run -d --name "$KN" --network "$NETZ" --env-file "$ENVDATEI" "$@" \
    -v pruefstand-wal:/data/wal -p 127.0.0.1:$PORT:8080 "aequitas-pruefstand:$NAME" >/dev/null
  for i in $(seq 1 60); do curl -fsS -m 3 "http://127.0.0.1:$PORT/api/status" >/dev/null 2>&1 && return 0; sleep 1; done
  echo "Pruefstand-Knoten antwortet nicht"; docker logs --tail 30 "$KN"; return 1
}
starte
ADDR="$(curl -fsS "http://127.0.0.1:$PORT/api/status" | grep -oE '"validator_address": ?"0x[0-9a-fA-F]{40}"' | grep -oE '0x[0-9a-fA-F]{40}')"
{ echo "AUTHORIZED_VALIDATORS=$ADDR"; echo "BOOTSTRAP_SIGNER=$ADDR"; } >> "$ENVDATEI"
starte
rm -f "$ENVDATEI"

echo "== $KONTEN Testkonten"
WERK="$QUELLE/tools/contabo-loadtest"
cp "$QUELLE/go.mod" "$QUELLE/go.sum" "$WERK/"
mkdir -p /root/go-mod-cache
docker run --rm -v "$WERK":/w -w /w -v /root/go-mod-cache:/go/pkg/mod golang:1.26.8-alpine \
  sh -c "go build -o loadtest main.go && go run gen_accounts.go $KONTEN > accounts.csv"
# Guthaben direkt in die Wegwerf-Datenbank: 100 AEQ (freie Adressen duerfen
# hoechstens 250 halten).
tail -n +2 "$WERK/accounts.csv" | cut -d, -f2 | tr 'A-F' 'a-f' \
  | awk -v t="$(date +%s)" -v q="'" '{printf "INSERT INTO chain_accounts(address,balance,last_activity_at) VALUES (%s%s%s,100,%s) ON CONFLICT (address) DO UPDATE SET balance=100;\n", q, $1, q, t}' \
  | docker exec -i "$PG" psql -q -U postgres -d aequitas

echo "== Last ($DAUER, Buendel $BUENDEL)"
docker run --rm --network host -v "$WERK":/w -w /w golang:1.26.8-alpine \
  ./loadtest -accounts accounts.csv -rpc "http://127.0.0.1:$PORT/rpc" -status "http://127.0.0.1:$PORT/api/status" \
    -phase warmup,run -duration "$DAUER" -batch-size "$BUENDEL" 2>&1 \
  | grep -vE '^warmup pair [0-9]+ ok|^\[monitor\]' | tail -25

echo "== Knoten nach dem Lauf"
curl -fsS "http://127.0.0.1:$PORT/api/health/combined" | python3 -c '
import json,sys
d=json.load(sys.stdin)
for k in ("produktion","produktion_phasen","eigenlast_bremse","rueckstau","inflight","wal_flush","wal_writer","leistungsnachweis"):
    v=d.get(k)
    if isinstance(v,dict): v={a:b for a,b in v.items() if a not in ("bedeutung","sync_verteilung")}
    print(k, json.dumps(v, ensure_ascii=False)[:900])
'
# Woher Konflikte kommen: nur Zeilen zu Versionskonflikten und den Zeilen
# davor (Wegwerf-Knoten; Schluessel stehen nicht in diesen Zeilen).
echo "== Versionskonflikte im Knotenprotokoll"
docker logs "$KN" 2>&1 | grep -ciE 'version conflict' || true
docker logs "$KN" 2>&1 | grep -iE -B3 'version conflict' | grep -viE 'key|secret|passw' | cut -c1-300 | head -24 || true
echo "== Box waehrend des Laufs"; uptime
