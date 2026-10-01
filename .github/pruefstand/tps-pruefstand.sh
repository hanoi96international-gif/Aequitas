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
# BUENDEL (Ueberweisungen je RPC-Buendel), PHASE (siehe unten).
#
# PHASE (seit 01.10.2026):
#   alles  -- wie bisher: aufbauen, Last von dieser Box, messen, abraeumen.
#   aufbau -- nur aufbauen und Konten anlegen; der Pruefknoten BLEIBT stehen
#             (127.0.0.1:18080). Die Last kommt dann von aussen, ueber einen
#             SSH-Tunnel von GitHub-Rechnern: so misst der Pruefstand den
#             Knoten, nicht Knoten + Generator auf denselben Kernen.
#   messen -- nur die Messungen (CPU, Profil, Zeitreihe, Goroutinen) ueber
#             ein festes Fenster, waehrend die Last von aussen laeuft.
#   abbau  -- alles entfernen.
# Der Pruefstand laeuft mit niedriger CPU-Prioritaet (--cpu-shares): bei
# Knappheit hat der laufende Knoten Vorrang, echte Nutzer merken nichts.
set -euo pipefail
PHASE="${PHASE:-alles}"
case "$PHASE" in alles|aufbau|messen|abbau) ;; *) echo "PHASE unzulaessig"; exit 1 ;; esac
NIEDRIG="--cpu-shares=256"
if [ "$PHASE" = abbau ]; then
  docker rm -f pruefstand-node pruefstand-pg pruefstand-last >/dev/null 2>&1 || true
  docker volume rm -f pruefstand-wal >/dev/null 2>&1 || true
  docker network rm pruefstand-net >/dev/null 2>&1 || true
  echo "Pruefstand abgeraeumt"; exit 0
fi
if [ "$PHASE" != messen ]; then : "${QUELLE:?}" "${NAME:?}"; fi
NAME="${NAME:-a}"
DAUER="${DAUER:-40s}"; KONTEN="${KONTEN:-1000}"; BUENDEL="${BUENDEL:-20}"
NETZ=pruefstand-net; PG=pruefstand-pg; KN=pruefstand-node; PORT=18080

ENVDATEI=""
aufraeumen() {
  [ -n "$ENVDATEI" ] && rm -f "$ENVDATEI"
  docker rm -f "$KN" "$PG" pruefstand-last >/dev/null 2>&1 || true
  docker volume rm -f pruefstand-wal >/dev/null 2>&1 || true
  docker network rm "$NETZ" >/dev/null 2>&1 || true
}
if [ "$PHASE" = alles ]; then trap aufraeumen EXIT; fi
if [ "$PHASE" = alles ] || [ "$PHASE" = aufbau ]; then
aufraeumen

echo "== Bauen ($NAME)"
docker build -q -t "aequitas-pruefstand:$NAME" "$QUELLE" >/dev/null
docker network create "$NETZ" >/dev/null

echo "== Postgres (wegwerfbar, dieselben Einstellungen wie deploy/validator)"
docker run -d --name "$PG" $NIEDRIG --network "$NETZ" -e POSTGRES_PASSWORD=pruefstand -e POSTGRES_DB=aequitas \
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
  # Wie deploy/validator/docker-compose.yml: keine Protokollzeile je Ueberweisung.
  echo "AEQUITAS_RPC_QUIET_TX=1"
  # Zusaetzliche Leistungsschalter fuer diesen Lauf (Workflow prueft die
  # Liste; hier noch einmal, fail closed).
  if [ -n "${EINSTELLUNGEN:-}" ]; then
    printf '%s\n' "$EINSTELLUNGEN" | tr ',' '\n' \
      | grep -E '^((AEQUITAS_WAL_FLUSH_(BATCH|CONCURRENCY|INTERVAL_MS)|AEQUITAS_WAL_QUEUE_DEPTH|AEQUITAS_DB_MAX_CONNS)=[0-9]{1,6}|AEQUITAS_BLOCK_AUS_SPEICHER=[01])$' || true
  fi
  # Der Generator kommt ueber den SSH-Tunnel und den veroeffentlichten Port,
  # beim Knoten also von EINER Adresse: dem Gateway des Pruefstand-Netzes.
  # Statt die Ratenbegrenzung fuer alle aufzudrehen (verboten: Schutzgrenzen
  # nie fuer Messungen lockern, AGENTS.md) wird nur diese Adresse
  # freigestellt -- derselbe Mechanismus wie in Produktion fuer den
  # Lastgenerator (rpc_frei.go). Inflight, Rueckstau und WAL-Druck gelten.
  GW="$(docker network inspect "$NETZ" -f '{{(index .IPAM.Config 0).Gateway}}')"
  [[ "$GW" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || { echo "Gateway des Pruefstand-Netzes unbekannt: '$GW'" >&2; exit 1; }
  echo "AEQUITAS_RPC_RATE_LIMIT_FREI=$GW"
} > "$ENVDATEI"

starte() {
  docker rm -f "$KN" >/dev/null 2>&1 || true
  docker run -d --name "$KN" $NIEDRIG --network "$NETZ" --env-file "$ENVDATEI" "$@" \
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
docker run --rm $NIEDRIG -v "$WERK":/w -w /w -v /root/go-mod-cache:/go/pkg/mod golang:1.26.8-alpine \
  sh -c "go build -o loadtest main.go && go run gen_accounts.go $KONTEN > accounts.csv"
# Guthaben direkt in die Wegwerf-Datenbank: 100 AEQ (freie Adressen duerfen
# hoechstens 250 halten).
tail -n +2 "$WERK/accounts.csv" | cut -d, -f2 | tr 'A-F' 'a-f' \
  | awk -v t="$(date +%s)" -v q="'" '{printf "INSERT INTO chain_accounts(address,balance,last_activity_at) VALUES (%s%s%s,100,%s) ON CONFLICT (address) DO UPDATE SET balance=100;\n", q, $1, q, t}' \
  | docker exec -i "$PG" psql -q -U postgres -d aequitas
# Fuer die Generatoren draussen: die Konten (Wegwerf-Schluessel eines
# Wegwerf-Knotens, der nur ueber 127.0.0.1 erreichbar ist).
cp "$WERK/accounts.csv" /root/pruefstand/konten.csv
fi
if [ "$PHASE" = aufbau ]; then
  # Aufwaermen HIER, lokal: jedes Konto einmal anfassen. Ueber den Tunnel
  # (0,3 s je Anfrage, nacheinander) dauerte das ~11 Minuten je Generator,
  # und die Lastfenster der Generatoren lagen dadurch versetzt.
  echo "== Aufwaermen (lokal, alle Konten)"
  docker run --rm --name pruefstand-last $NIEDRIG --network host -v "$WERK":/w -w /w golang:1.26.8-alpine \
    ./loadtest -accounts accounts.csv -rpc "http://127.0.0.1:$PORT/rpc" -status "http://127.0.0.1:$PORT/api/status" \
      -phase warmup 2>&1 | grep -vE '^warmup pair [0-9]+ ok|^\[monitor\]' | tail -5
  echo "Pruefknoten steht: 127.0.0.1:$PORT, $KONTEN Konten, aufgewaermt"; exit 0
fi
docker inspect "$KN" >/dev/null 2>&1 || { echo "Pruefknoten laeuft nicht"; exit 1; }

echo "== Last ($DAUER, Buendel $BUENDEL, Phase $PHASE)"
# CPU-Profil aus dem Messfenster (Aufwaermen dauert rund 50 s). pprof lauscht
# nur auf 127.0.0.1 im Container und ist von aussen nicht erreichbar.
PROFIL="$(mktemp -d)"
# Bei Last von aussen (PHASE=messen) beginnt die Last ~5 s nach dem
# Messstart; beim Lauf von der Box (alles) erst nach dem Aufwaermen.
PROFIL_NACH=70; [ "$PHASE" = messen ] && PROFIL_NACH=15
( sleep "$PROFIL_NACH"; docker exec "$KN" wget -qO- 'http://127.0.0.1:6061/debug/pprof/profile?seconds=20' > "$PROFIL/cpu.pb" 2>/dev/null || true ) &
PROFIL_PID=$!
# Worauf warten die Goroutinen? Schnappschuss mitten im Messfenster,
# gruppiert nach identischem Stapel (debug=1), die groessten Gruppen.
( beste=0
  for i in $(seq 1 60); do
    docker exec "$KN" wget -qO- 'http://127.0.0.1:6061/debug/pprof/goroutine?debug=1' > "$PROFIL/gr.neu" 2>/dev/null || true
    n="$(head -1 "$PROFIL/gr.neu" 2>/dev/null | grep -oE '[0-9]+$' || echo 0)"
    if [ "${n:-0}" -gt "$beste" ]; then beste="$n"; mv "$PROFIL/gr.neu" "$PROFIL/gr.txt"; fi
    sleep 2
  done ) &
GR_PID=$!
# Zeitreihe alle 2 s: wo staut es sich? (Rueckstand gesamt, davon noch im WAL,
# offen in pending_txs, Inflight, Hoehe.) Nur Zahlen.
( for i in $(seq 1 60); do
    h="$(curl -s -m 2 "http://127.0.0.1:$PORT/api/health/combined" 2>/dev/null || true)"
    p="$(docker exec "$PG" psql -tA -U postgres -d aequitas -c "SELECT count(*) FILTER (WHERE included_at=0) || '/' || count(*) FROM pending_txs" 2>/dev/null || true)"
    printf '%s' "$h" | python3 -c '
import json,sys
try: d=json.load(sys.stdin)
except Exception: sys.exit(0)
r=d.get("rueckstau",{}); i=d.get("inflight",{})
print("[reihe] t=%s rueckstau=%s gemessen=%s wal=%s pending_offen/gesamt=%s inflight=%s" % (sys.argv[1], r.get("aktuell"), r.get("gemessen"), d.get("wal_warteschlange"), sys.argv[2], i.get("aktuell")))
' "$((i*2))" "$p" || true
    sleep 2
  done ) > "$PROFIL/reihe.txt" 2>&1 &
REIHE_PID=$!
# CPU je Container (Prozent eines Kerns), alle ~3 s: Knoten, Postgres,
# Lastgenerator und der laufende Knoten daneben. Daraus: Kerne je 10.000
# Ueberweisungen/s -- was ein eigener Validator ohne Generator schafft.
# CPU je Container aus den Kernel-Zaehlern (cgroup v2, cpu.stat usage_usec):
# Stand am Anfang und am Ende des Lastfensters, daraus Kerne im Mittel.
# (docker stats lieferte unter Last keine verwertbaren Proben.)
cg_usec() {
  local id; id="$(docker inspect -f '{{.Id}}' "$1" 2>/dev/null)" || return 0
  for f in "/sys/fs/cgroup/system.slice/docker-$id.scope/cpu.stat" "/sys/fs/cgroup/docker/$id/cpu.stat"; do
    [ -r "$f" ] && { awk '/^usage_usec/{print $2}' "$f"; return 0; }
  done
}
FENSTER=45; [ "$PHASE" = alles ] && FENSTER=60
( sleep 5
  for c in "$KN" "$PG" aequitas-node; do echo "$c $(cg_usec "$c") $(date +%s%N)"; done > "$PROFIL/cpu_a.txt"
  sleep "$FENSTER"
  for c in "$KN" "$PG" aequitas-node; do echo "$c $(cg_usec "$c") $(date +%s%N)"; done > "$PROFIL/cpu_b.txt"
) > /dev/null 2>&1 &
CPU_PID=$!
if [ "$PHASE" = alles ]; then
docker run --rm --name pruefstand-last $NIEDRIG --network host -v "$WERK":/w -w /w golang:1.26.8-alpine \
  ./loadtest -accounts accounts.csv -rpc "http://127.0.0.1:$PORT/rpc" -status "http://127.0.0.1:$PORT/api/status" \
    -phase warmup,run -duration "$DAUER" -batch-size "$BUENDEL" 2>&1 \
  | grep -vE '^warmup pair [0-9]+ ok|^\[monitor\]' | tail -25
else
  # Last kommt von aussen; die Messfenster oben laufen 120 s.
  sleep 120
fi

wait "$PROFIL_PID" 2>/dev/null || true
kill "$REIHE_PID" 2>/dev/null || true
echo "== CPU je Container im Lastfenster (Kerne im Mittel, aus cgroup cpu.stat)"
wait "$CPU_PID" 2>/dev/null || true
python3 - "$PROFIL/cpu_a.txt" "$PROFIL/cpu_b.txt" <<'PY' || true
import sys
def lies(p):
    d={}
    for z in open(p):
        t=z.split()
        if len(t)==3 and t[1].isdigit(): d[t[0]]=(int(t[1]),int(t[2]))
    return d
a,b=lies(sys.argv[1]),lies(sys.argv[2])
for k in a:
    if k in b:
        dt=(b[k][1]-a[k][1])/1e9
        print("  %-18s %5.2f Kerne" % (k,(b[k][0]-a[k][0])/1e6/max(dt,1e-9)))
PY
echo "== Zeitreihe (alle 2 s ab Lastbeginn)"
cat "$PROFIL/reihe.txt" 2>/dev/null | head -60 || true
wait "$GR_PID" 2>/dev/null || true
echo "== Goroutinen im Messfenster (Schnappschuss mit den meisten Goroutinen; groesste Gruppen gleicher Stapel)"
python3 - "$PROFIL/gr.txt" <<'PY' || true
import re,sys
try: t=open(sys.argv[1]).read()
except Exception: print("(kein Schnappschuss)"); sys.exit(0)
gruppen=[]
for block in t.split("\n\n"):
    z=block.strip().splitlines()
    if not z: continue
    m=re.match(r"(\d+) @",z[0])
    if not m: continue
    fn=[re.split(r"\s+",l.strip())[2] for l in z[1:] if l.strip().startswith("#") and len(re.split(r"\s+",l.strip()))>2]
    fn=[f.split("/")[-1] for f in fn]
    gruppen.append((int(m.group(1)),fn))
gruppen.sort(key=lambda g:-g[0])
print("gesamt:", sum(g[0] for g in gruppen))
for n,fn in gruppen[:14]:
    print("%6d  %s" % (n, " <- ".join(fn[:9])))
PY
echo "== CPU-Profil (20 s im Messfenster, oberste Posten)"
if [ -s "$PROFIL/cpu.pb" ]; then
  docker run --rm -v "$PROFIL":/p golang:1.26.8-alpine go tool pprof -top -nodecount=30 /p/cpu.pb 2>/dev/null | tail -32 || true
  docker run --rm -v "$PROFIL":/p golang:1.26.8-alpine go tool pprof -top -cum -nodecount=30 /p/cpu.pb 2>/dev/null | tail -30 || true
else
  echo "(kein Profil)"
fi
rm -rf "$PROFIL"
echo "== Knoten nach dem Lauf"
curl -fsS "http://127.0.0.1:$PORT/api/health/combined" | python3 -c '
import json,sys
d=json.load(sys.stdin)
for k in ("produktion","produktion_phasen","produktions_ausfaelle","speicherkorb","eigenlast_bremse","peer_lag_bremse","rueckstau","inflight","wal_druck","fallback_gruende","wal_flush","wal_writer","leistungsnachweis"):
    v=d.get(k)
    if isinstance(v,dict): v={a:b for a,b in v.items() if a not in ("bedeutung","sync_verteilung")}
    print(k, json.dumps(v, ensure_ascii=False)[:900])
'
# Je Produktionsversuch (produktion_protokoll.go): zeigt, ob Zusatzbloecke
# je Takt entstehen, welcher Deckel galt und wo die Zeit eines langsamen
# Blocks blieb. Nur Zahlen, keine Inhalte.
echo "== Produktionsversuche (letzte 160; ms relativ zum ersten)"
curl -fsS "http://127.0.0.1:$PORT/api/produktion?n=160" | python3 -c '
import json,sys
d=json.load(sys.stdin)
e=d.get("eintraege", d) if isinstance(d,dict) else d
e=[x for x in e if isinstance(x,dict)]
if not e: print("(leer)"); sys.exit()
t0=e[0].get("at_ms",0)
print("     t_ms  txs  deckel  gesamt  laden  sperren  db_paar  speichern  grund")
for x in e:
    print("%9d %5d %6d %7.0f %6.0f %8.0f %8.0f %10.0f  %s" % (x.get("at_ms",0)-t0, x.get("txs",0), x.get("deckel",0), x.get("gesamt_ms",0), x.get("laden_ms",0), x.get("sperren_ms",0), x.get("db_paar_ms",0), x.get("speichern_ms",0), x.get("grund","")))
' || echo "(nicht lesbar)"
echo "== Zusatzbloecke je Takt (Knotenprotokoll)"
docker logs "$KN" 2>&1 | grep -oE 'produced [0-9]+ blocks this tick' | sort | uniq -c || true
docker logs "$KN" 2>&1 | grep -c 'Full tick (ProduceBlock+broadcast) took' || true
# Woher Konflikte kommen: nur Zeilen zu Versionskonflikten und den Zeilen
# davor (Wegwerf-Knoten; Schluessel stehen nicht in diesen Zeilen).
echo "== Versionskonflikte im Knotenprotokoll"
docker logs "$KN" 2>&1 | grep -ciE 'version conflict' || true
docker logs "$KN" 2>&1 | grep -iE -B3 'version conflict' | grep -viE 'key|secret|passw' | cut -c1-300 | head -24 || true
echo "== Box waehrend des Laufs"; uptime
