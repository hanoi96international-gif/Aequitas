# UNUMKEHRBAR: leert auf dieser Box Kette, WAL, Proof-Server und Matching
# fuer den Neustart bei null. Der Knoten bleibt danach GESTOPPT; starten.sh
# startet ihn (erst C1, dann C2). Vorher: backup-ledger.sh (Workflow).
set -euo pipefail
[ "${BESTAETIGT:-}" = "ALLE GUTHABEN WEG" ] || { echo "ABBRUCH: keine Bestaetigung"; exit 1; }

# ── Erst alles pruefen, dann loeschen: ein unbekannter Aufbau bricht ab,
#    bevor etwas weg ist.
fehler() { echo "ABBRUCH vor dem Loeschen: $*"; exit 1; }
docker inspect "$NODE" >/dev/null 2>&1 || fehler "kein Container $NODE"
laeuft "$PG" || fehler "$PG laeuft nicht"
[[ "$DB" =~ ^[a-z_][a-z0-9_]*$ ]] || fehler "Datenbankname unlesbar"
[[ "$DBUSER" =~ ^[a-z_][a-z0-9_]*$ ]] || fehler "Datenbanknutzer unlesbar"
case "$DBHOST" in postgres|aequitas-postgres) ;; *) fehler "Datenbank-Host '$DBHOST' ist nicht $PG";; esac
[ "$DB" != postgres ] || fehler "die Knoten-Datenbank heisst 'postgres' -- die wird nicht geloescht"
[ -n "$WALDIR" ] && [ "$WALDIR" != / ] && [ -d "$WALDIR" ] || fehler "WAL-Verzeichnis unklar ('$WALDIR')"
case "$WALDIR" in /root/*|/var/lib/docker/volumes/*) ;; *) fehler "WAL-Verzeichnis '$WALDIR' liegt unerwartet";; esac
if [ -n "$MATCH" ]; then laeuft "$MATCH" || fehler "$MATCH laeuft nicht"; fi
if docker inspect "$PROOF" >/dev/null 2>&1; then laeuft "$PROOF" || fehler "$PROOF laeuft nicht"; fi
echo "Geprueft: Knoten=$NODE DB=$DB WAL=$WALDIR Proof=$PROOF Matching=${MATCH:-keiner}"

# ── Ab hier wird geloescht.
echo "=== Knoten anhalten ==="
docker stop -t 60 "$NODE" >/dev/null
FEHLER=0

echo "=== Kettendatenbank $DB neu anlegen ==="
docker exec "$PG" psql -U "$DBUSER" -d postgres -v ON_ERROR_STOP=1 \
  -c "DROP DATABASE \"$DB\" WITH (FORCE)" \
  -c "CREATE DATABASE \"$DB\"" || { echo "FEHLER: Datenbank"; FEHLER=1; }

echo "=== WAL leeren ==="
find "$WALDIR" -mindepth 1 -delete || { echo "FEHLER: WAL"; FEHLER=1; }
echo "WAL jetzt: $(find "$WALDIR" -mindepth 1 | wc -l) Eintraege"

if laeuft "$PROOF"; then
  echo "=== Proof-Server-Tabellen leeren ==="
  docker exec -i -e TABELLEN="$PROOF_TABELLEN" "$PROOF" node - <<'JS' || { echo "FEHLER: Proof-Server"; FEHLER=1; }
const { Pool } = require('pg');
const pool = new Pool({ connectionString: process.env.DATABASE_URL, ssl: { rejectUnauthorized: process.env.DB_SSL_REJECT_UNAUTHORIZED !== 'false' } });
(async () => {
  const da = [];
  for (const t of process.env.TABELLEN.split(' ')) {
    const e = await pool.query('select to_regclass($1) as r', [t]);
    if (e.rows[0].r) da.push(t);
  }
  if (da.length) await pool.query('TRUNCATE ' + da.join(', '));
  for (const t of da) {
    const n = await pool.query('select count(*)::int as n from ' + t);
    console.log('   ' + t + ' jetzt ' + n.rows[0].n);
  }
  await pool.end();
})().catch(e => { console.error('   ' + e.message); process.exit(1); });
JS
fi

if [ -n "$MATCH" ]; then
  echo "=== Matching-Datenbank leeren ==="
  docker exec -i "$MATCH" python3 - <<'PY' || { echo "FEHLER: Matching"; FEHLER=1; }
import os, sqlite3
p = os.environ.get("POH_DB_PATH", "/app/data/poh.db")
c = sqlite3.connect(p)
ts = [t for (t,) in c.execute("select name from sqlite_master where type='table' and name not like 'sqlite_%'")]
for t in ts:
    c.execute('delete from "%s"' % t)
c.commit()
c.execute("vacuum")
for t in ts:
    print("   %s jetzt %d" % (t, c.execute('select count(*) from "%s"' % t).fetchone()[0]))
PY
  docker restart "$MATCH" >/dev/null
fi

if [ "$FEHLER" != 0 ]; then
  echo "::error::Mindestens ein Schritt ist gescheitert (siehe oben). Der Knoten bleibt gestoppt."
  exit 1
fi
echo "=== $BOX geleert. Knoten gestoppt, wartet auf starten.sh. ==="
