# Gemeinsame Ermittlung fuer erkunden.sh, leeren.sh und starten.sh; wird auf
# der Box vor das eigentliche Skript gehaengt. Liest nur Container-Metadaten
# und gibt keine Zugangsdaten aus (das Repo und die Logs sind oeffentlich).
NODE=aequitas-node
PG=aequitas-postgres
PROOF=aequitas-proof-server
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
PROOF_TABELLEN="bio_hashes bio_hashes_dedup_backup bio_hashes_clear_backup proof_store prove_locks rate_limits"
