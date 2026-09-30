# NUR LESEN. Zeigt, was leeren.sh auf dieser Box anfassen wuerde.
# Ausgabe: Namen, Pfade, Groessen, Zeilenzahlen -- keine Inhalte, keine Werte.
set -u
echo "===== $BOX: Container ====="
docker ps -a --format '{{.Names}}\t{{.Status}}\t{{.Image}}' | sort
echo
echo "===== $BOX: Mounts ====="
for c in $(docker ps -a --format '{{.Names}}' | sort); do
  echo "-- $c"
  docker inspect "$c" --format '{{range .Mounts}}   {{.Type}} {{if .Name}}{{.Name}}{{else}}{{.Source}}{{end}} -> {{.Destination}}{{println}}{{end}}'
done
echo
echo "===== $BOX: was leeren.sh nehmen wuerde ====="
echo "Knoten:        $NODE ($(docker inspect "$NODE" --format '{{.State.Status}}' 2>/dev/null || echo fehlt))"
echo "Postgres:      $PG ($(docker inspect "$PG" --format '{{.State.Status}}' 2>/dev/null || echo fehlt))"
echo "Ketten-DB:     Name=$DB Nutzer=$DBUSER Host=$DBHOST"
echo "WAL-Verz.:     ${WALDIR:-FEHLT} ($([ -n "$WALDIR" ] && du -sh "$WALDIR" 2>/dev/null | cut -f1))"
echo "Proof-Server:  $PROOF DB=${PROOF_DB:-?} ($(docker inspect "$PROOF" --format '{{.State.Status}}' 2>/dev/null || echo fehlt))"
echo "Matching:      ${MATCH:-FEHLT} POH_DB_PATH=$([ -n "$MATCH" ] && env_von "$MATCH" POH_DB_PATH)"
echo "Coordinator:   ${COORD:-FEHLT} (bleibt, wird nur neu gestartet)"
echo
echo "===== $BOX: Postgres-Datenbanken ====="
docker exec "$PG" psql -U "${DBUSER:-postgres}" -d postgres -Atc \
  "select '   '||datname||' '||pg_size_pretty(pg_database_size(datname)) from pg_database where not datistemplate" 2>&1
for d in $(printf '%s\n' "$DB" "$PROOF_DB" | sort -u); do
  [ -n "$d" ] || continue
  echo "-- $d: Tabellen mit Zeilen (Schaetzung)"
  docker exec "$PG" psql -U "${DBUSER:-postgres}" -d "$d" -Atc \
    "select '   '||relname||' ~'||n_live_tup from pg_stat_user_tables where n_live_tup>0 order by n_live_tup desc limit 30" 2>&1
done
echo
echo "===== $BOX: Proof-Server-Tabellen (genau) ====="
if laeuft "$PROOF"; then
  docker exec -i -e TABELLEN="$PROOF_TABELLEN" "$PROOF" node - <<'JS' 2>&1 || true
const { Pool } = require('pg');
const pool = new Pool({ connectionString: process.env.DATABASE_URL, ssl: { rejectUnauthorized: process.env.DB_SSL_REJECT_UNAUTHORIZED !== 'false' } });
(async () => {
  for (const t of process.env.TABELLEN.split(' ')) {
    const e = await pool.query('select to_regclass($1) as r', [t]);
    if (!e.rows[0].r) { console.log('   ' + t + ' (fehlt)'); continue; }
    const n = await pool.query('select count(*)::int as n from ' + t);
    console.log('   ' + t + ' ' + n.rows[0].n);
  }
  await pool.end();
})().catch(e => { console.log('   Fehler: ' + e.message); });
JS
else
  echo "   (laeuft nicht)"
fi
echo
echo "===== $BOX: Dateien von Matching und Coordinator ====="
for c in $MATCH $COORD; do
  echo "-- $c"
  docker exec -i "$c" python3 - <<'PY' 2>&1 || true
import os, sqlite3
for d in ("/app/data", "/data"):
    for w, _, fs in os.walk(d):
        for f in fs:
            p = os.path.join(w, f)
            print("   %s %d Byte" % (p, os.path.getsize(p)))
            if f.endswith((".db", ".sqlite", ".sqlite3")):
                c = sqlite3.connect("file:%s?mode=ro" % p, uri=True)
                for (t,) in c.execute("select name from sqlite_master where type='table' and name not like 'sqlite_%'"):
                    print("      %s %d" % (t, c.execute('select count(*) from "%s"' % t).fetchone()[0]))
PY
done
echo
echo "===== $BOX: Genesis und Status des laufenden Knotens ====="
docker exec "$NODE" cat genesis.json 2>&1
echo
curl -s -m 5 http://127.0.0.1:8080/api/status | grep -oE '"(height|total_humans|netz_kennung|register_vertrag)": ?("[^"]*"|[0-9]+)' | tr '\n' ' '
echo
echo
echo "===== $BOX: Validator- und MPC-Eintraege (nur Adressen, URLs, Schalter) ====="
# Fuer den Schluesselwechsel beim Neustart: wer steht wo. Private Schluessel
# werden nie gelesen; von RELAYER_PRIVATE_KEY nur, ob gesetzt, und die
# oeffentliche Adresse aus /api/status.
for v in AUTHORIZED_VALIDATORS MPC_PEERS MPC_ENABLED MPC_REQUIRED MPC_PARTY_INDEX RELAYER_ADDRESS NODE_OPERATOR_WALLET PRIMARY_NODE_URLS ANNAHME_ROLLE; do
  w="$(env_von "$NODE" "$v" | grep -oE '0x[0-9a-fA-F]{40}|https?://[A-Za-z0-9.:/_-]+|\b(true|false|[0-9]+|annehmend|nur_lesend|keine)\b' | tr '\n' ' ')"
  echo "$v = ${w:-(leer)}"
done
echo "RELAYER_PRIVATE_KEY gesetzt: $([ -n "$(env_von "$NODE" RELAYER_PRIVATE_KEY)" ] && echo ja || echo nein)"
echo "NODE_OPERATOR_BINDING_SIGNATURE gesetzt: $([ -n "$(env_von "$NODE" NODE_OPERATOR_BINDING_SIGNATURE)" ] && echo ja || echo nein)"
curl -s -m 5 http://127.0.0.1:8080/api/status | grep -oE '"(validator_address|produzenten_geschlossen)": ?("[^"]*"|true|false)' | tr '\n' ' '
echo
for f in /root/.aequitas.env /root/Aequitas/deploy/validator/.env; do
  [ -f "$f" ] && echo "Umgebungsdatei: $f ($(grep -cE '^[A-Z_]+=' "$f") Eintraege, RELAYER_PRIVATE_KEY-Zeilen: $(grep -c '^RELAYER_PRIVATE_KEY=' "$f"))"
done
echo
echo "===== $BOX: Deploy-Skript der Box (nur ob es baut) ====="
for f in /root/deploy_safe_c2.sh /root/Aequitas/deploy/deploy-c1.sh; do
  [ -f "$f" ] && echo "$f: git fetch/reset/pull $(grep -cE 'git (fetch|reset|pull)' "$f")x, docker build $(grep -cE 'docker (compose )?build|docker build' "$f")x, docker run/up $(grep -cE 'docker run|compose up' "$f")x"
done
echo
echo "===== $BOX: Platte und Sicherungen ====="
df -h / | tail -1
ls -la /root/backups 2>/dev/null | tail -3
