# Gemeinsame Ermittlung fuer erkunden.sh, leeren.sh und starten.sh; wird auf
# der Box vor das eigentliche Skript gehaengt. Liest nur Container-Metadaten
# und gibt keine Zugangsdaten aus (das Repo und die Logs sind oeffentlich).
NODE=aequitas-node
PG=aequitas-postgres
PROOF="$(docker ps -a --format '{{.Names}}' | grep -E '^(aequitas-)?proof-server$' | head -1 || true)"
MATCH="$(docker ps -a --format '{{.Names}}' | grep -E '^aequitas-matching(-[0-9]+)?$' | head -1 || true)"
COORD="$(docker ps -a --format '{{.Names}}' | grep -E '^aequitas-coordinator(-[0-9]+)?$' | head -1 || true)"
env_von() { docker inspect "$1" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E "^$2=" | head -1 | cut -d= -f2- || true; }
laeuft() { [ "$(docker inspect "$1" --format '{{.State.Running}}' 2>/dev/null)" = true ]; }
URL="$(grep -E '^DATABASE_URL=' /root/.aequitas.env 2>/dev/null | head -1 | cut -d= -f2- || true)"
[ -n "$URL" ] || URL="$(env_von "$NODE" DATABASE_URL)"
# postgres://nutzer:pass@host:port/db?opts -> nur Name, Nutzer, Host
DB="${URL##*/}"; DB="${DB%%\?*}"
REST="${URL#*://}"; DBUSER="${REST%%@*}"; DBUSER="${DBUSER%%:*}"
DBHOST="${REST#*@}"; DBHOST="${DBHOST%%/*}"; DBHOST="${DBHOST%%:*}"
WALDIR="$(docker inspect "$NODE" --format '{{range .Mounts}}{{if eq .Destination "/data/wal"}}{{.Source}}{{end}}{{end}}' 2>/dev/null || true)"
PROOF_URL="$(env_von "$PROOF" DATABASE_URL)"
PROOF_DB="${PROOF_URL##*/}"; PROOF_DB="${PROOF_DB%%\?*}"
PROOF_TABELLEN="bio_hashes bio_hashes_dedup_backup bio_hashes_clear_backup proof_store prove_locks rate_limits einzelbetrieb_plaetze"
# Die Matching-Datenbank: nur der Pfad aus dem Container, nie ein Rueckfall.
# Mit einem falschen Pfad haette leeren.sh eine neue, leere Datei "geleert"
# und die alten Gesichter behalten -- nach dem Neustart galt man als Duplikat.
MATCH_DB="$([ -n "$MATCH" ] && env_von "$MATCH" POH_DB_PATH)"
# registrierungen_zaehlen: druckt "matching_test N", "matching_real N",
# "matching_sicherungen N" und "proof_bio_hashes N"; Fehler beim Lesen -> "?".
registrierungen_zaehlen() {
  docker exec -i -e P="$MATCH_DB" "$MATCH" python3 - <<'PY' || echo "matching ?"
import glob, os, sqlite3
p = os.environ["P"]
c = sqlite3.connect("file:" + p + "?mode=ro", uri=True)
for t, n in (("test_enrollments", "matching_test"), ("real_enrollments", "matching_real")):
    try:
        print(n, c.execute("select count(*) from " + t).fetchone()[0])
    except sqlite3.OperationalError:
        print(n, "?")
print("matching_sicherungen", len(glob.glob(glob.escape(p) + ".vor-*")))
PY
  docker exec -i "$PROOF" node -e 'const {Pool}=require("pg");const p=new Pool({connectionString:process.env.DATABASE_URL,ssl:{rejectUnauthorized:process.env.DB_SSL_REJECT_UNAUTHORIZED!=="false"}});p.query("select count(*)::int as n from bio_hashes").then(r=>{console.log("proof_bio_hashes",r.rows[0].n);return p.end()}).catch(()=>{console.log("proof_bio_hashes ?");process.exit(0)})' || echo "proof_bio_hashes ?"
}
# alles_null: liest registrierungen_zaehlen und scheitert, wenn ein Wert nicht 0 ist.
alles_null() {
  local z; z="$(registrierungen_zaehlen)"; echo "$z" | sed "s/^/   /"
  ! echo "$z" | awk '$2 != "0" {f=1} END {exit !f}'
}
