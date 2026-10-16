#!/usr/bin/env bash
# Biometrie- und Proof-Datenbanken von C1 sichern -- VERSCHLUESSELT AUF DER BOX.
#
# Laeuft ueber .github/workflows/backup-biometrie-verschluesselt.yml auf C1
# selbst. BIO_BACKUP_AGE_RECIPIENT (ein oeffentlicher age-Schluessel,
# age1...) kommt als export-Zeile vor dem Skript ueber stdin, nie auf der
# Befehlszeile, die ps zeigt (Muster: deploy/wachhund-einrichten.sh).
#
# WARUM EIN EIGENES BACKUP. deploy/backup-ledger.sh lagert den Zustand der
# Kette aus, aber absichtlich OHNE die Zeilen der biometrischen Tabellen: das
# Repo ist oeffentlich, jedes Artefakt fuer jeden angemeldeten GitHub-Nutzer
# lesbar (26.09.2026). Diese Daten lagen damit nur auf C1
# (docs/BETA_BEREITSCHAFT.md, Betreiber-Punkt 5).
#
# WAS GESICHERT WIRD (Fundorte, Stand 16.10.2026)
#
#   kette-bio.dump       Postgres des Knotens (DATABASE_URL von aequitas-node):
#                        mpc_shares, bio_hashes, bio_registrations,
#                        bio_registrations_bio_hash_dedup_backup,
#                        registration_recovery -- genau die Tabellen, deren
#                        Zeilen backup-ledger.sh weglaesst.
#   proof.dump           Container proof-postgres (postgres:16, Volume
#                        proof_pgdata), Datenbank aequitas_proof:
#                        bio_hashes, proof_store, einzelbetrieb_plaetze, ...
#                        (aequitas-proof-server, bio_store.js;
#                        deploy/neuer-c1-einrichten.sh dort).
#   coordinator/*.db     SQLite des Coordinators (aequitas-coordinator, /data
#                        = /root/coordinator-daten): widersprueche.db,
#                        einzelbetrieb.db, altersbuergschaften.db
#                        (aequitas-biometric-beta, coordinator-neuer-c1.yml).
#   matching/*.db        SQLite des Vergleichsdienstes (aequitas-matching,
#                        /data = /root/aequitas-matching-data): poh_beta.db,
#                        die Template-Galerie (vergleichsdienst-neuer-c1.yml).
#                        Die Templates darin sind mit TEMPLATE_ENCRYPTION_KEY
#                        versiegelt; der Schluessel gehoert NICHT in diese
#                        Sicherung, er wird getrennt offline verwahrt.
#
# ABLAUF. Jede Quelle wird fuer sich konsistent kopiert (pg_dump = ein
# Snapshot; SQLite ueber die Online-Backup-API), jede Kopie wird geprueft
# (Postgres: Wiederherstellung in einen Wegwerf-Postgres ohne Netz auf
# tmpfs; SQLite: PRAGMA integrity_check), dann wird alles als ein tar-Strom
# direkt in age geschrieben. Klartext gibt es nur im Arbeitsverzeichnis
# (0700) auf der Box; es wird nach dem Verschluesseln geloescht, und ein trap
# loescht es auch bei jedem Abbruch. Abgeholt wird nur die .age-Datei.
# Ueber die Quellen hinweg ist die Sicherung NICHT atomar (vier Datenbanken,
# vier Zeitpunkte innerhalb weniger Sekunden).
set -euo pipefail
umask 077

# --- 1. Empfaenger pruefen, BEVOR irgendetwas angefasst wird (fail-closed) ---
R="${BIO_BACKUP_AGE_RECIPIENT:-}"
if [ -z "$R" ]; then
  echo "::error::BIO_BACKUP_AGE_RECIPIENT fehlt -- ohne oeffentlichen age-Schluessel wird nichts gesichert (docs/BIOMETRIE_SICHERUNG.md)."
  exit 2
fi
# X25519-Empfaenger: age1 + 58 Zeichen Bech32 (ohne 1, b, i, o).
# Ganzer Wert, nicht zeilenweise (grep -x liesse "age1...\nanderes" durch).
R_REST="${R#age1}"
if [ "$R_REST" = "$R" ] || [ "${#R}" -ne 62 ] || case "$R_REST" in *[!qpzry9x8gf2tvdw0s3jn54khce6mua7l]*) true ;; *) false ;; esac; then
  echo "::error::BIO_BACKUP_AGE_RECIPIENT ist kein gueltiger oeffentlicher age-Schluessel (age1..., 62 Zeichen). Nichts gesichert (docs/BIOMETRIE_SICHERUNG.md)."
  exit 2
fi

BASIS=/root/bio-sicherung
ZIEL="$BASIS/bio-latest.tar.age"
SUMMARY="$BASIS/bio-latest.summary"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
WORK="$BASIS/klartext.$STAMP"
PRUEF="bio-restore-pruefung-$STAMP"
PG_IMAGE=postgres:16-alpine
# Grenzen (AGENTS.md Frage 2).
MAX_PG_BYTES=$((300 * 1024 * 1024))     # je Postgres-Dump (komprimiert)
MAX_KLARTEXT=$((1024 * 1024 * 1024))    # alles zusammen vor dem Verschluesseln
MAX_AGE_BYTES=$((500 * 1024 * 1024))    # die .age-Datei (Artefakt-Kontingent)
MIN_FREI_KB=$((2 * 1024 * 1024))        # freier Platz auf /root vor dem Start

aufraeumen() {
  docker rm -f "$PRUEF" >/dev/null 2>&1 || true
  rm -rf "$WORK" "$ZIEL.tmp"
}
trap aufraeumen EXIT INT TERM

mkdir -p "$BASIS" && chmod 700 "$BASIS"
# Eine Datei aus einem frueheren Lauf darf nicht als die heutige gelten.
rm -f "$ZIEL" "$SUMMARY"
# Klartext abgebrochener Laeufe (Zeitlimit, kill -9 umgeht den trap).
find "$BASIS" -maxdepth 1 -type d -name 'klartext.*' -exec rm -rf {} + 2>/dev/null || true
for alt in $(docker ps -a --format '{{.Names}}' | grep -E '^bio-restore-pruefung-' || true); do
  docker rm -f "$alt" >/dev/null 2>&1 || true
done

FREI_KB="$(df -Pk "$BASIS" | awk 'NR==2 {print $4}')"
case "$FREI_KB" in ''|*[!0-9]*) echo "::error::Freier Platz unlesbar. Abbruch."; exit 1 ;; esac
if [ "$FREI_KB" -lt "$MIN_FREI_KB" ]; then
  echo "::error::Nur ${FREI_KB} KB frei auf $BASIS (< 2 GB) -- keine Sicherung, die den Validator an den Plattenrand bringt."
  exit 1
fi

command -v python3 >/dev/null || { echo "::error::python3 fehlt auf der Box (SQLite-Backup-API)."; exit 1; }
# age aus dem Paket der Distribution, nicht per curl|sh.
if ! command -v age >/dev/null; then
  echo "age fehlt -- installiere das Distributionspaket"
  DEBIAN_FRONTEND=noninteractive timeout 300 apt-get update -qq >/dev/null
  DEBIAN_FRONTEND=noninteractive timeout 300 apt-get install -y -qq --no-install-recommends age >/dev/null
fi
command -v age >/dev/null || { echo "::error::age nicht installierbar."; exit 1; }
echo "age: $(age --version 2>/dev/null | head -1)"

mkdir -m 700 "$WORK" "$WORK/coordinator" "$WORK/matching"

# --- 2a. Biometrische Tabellen der Kette -------------------------------------
DATABASE_URL="$(docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E '^DATABASE_URL=' | head -1 | cut -d= -f2- || true)"
if [ -z "$DATABASE_URL" ]; then
  DATABASE_URL="$(grep -E '^DATABASE_URL=' /root/.aequitas.env 2>/dev/null | head -1 | cut -d= -f2- || true)"
fi
[ -n "$DATABASE_URL" ] || { echo "::error::Keine DATABASE_URL des Knotens. Abbruch."; exit 1; }
export DATABASE_URL
NET="$(docker inspect aequitas-node --format '{{range $n,$v := .NetworkSettings.Networks}}{{$n}}{{end}}')"
# Ohne Benutzer und Passwort ins Log.
echo "kette: $(printf '%s' "$DATABASE_URL" | sed -E 's#^[a-z]+://([^@]*@)?##; s#\?.*$##')"
# DATABASE_URL geht per -e NAME aus der Umgebung in den Container, nicht als
# Argument -- sonst stuende das Passwort in ps.
kette() { docker run --rm --network "$NET" -e DATABASE_URL "$PG_IMAGE" sh -c 'exec psql "$DATABASE_URL" -t -A -v ON_ERROR_STOP=1 -c "$1"' sh "$1"; }

BIO_TABELLEN="mpc_shares bio_hashes bio_registrations bio_registrations_bio_hash_dedup_backup registration_recovery"
VORHANDEN="$(kette "SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename IN ('$(echo "$BIO_TABELLEN" | sed "s/ /','/g")') ORDER BY 1")"
echo "$VORHANDEN" | grep -qx bio_hashes || { echo "::error::Tabelle bio_hashes fehlt in der Datenbank des Knotens -- falsche Datenbank? Abbruch."; exit 1; }
LIVE_KETTE_BIO="$(kette "SELECT count(*) FROM bio_hashes")"
case "$LIVE_KETTE_BIO" in ''|*[!0-9]*) echo "::error::Zaehlung bio_hashes (Kette) unlesbar. Abbruch."; exit 1 ;; esac
T_ARGS=()
for t in $VORHANDEN; do T_ARGS+=(-t "public.$t"); done
echo "kette: Tabellen $(echo $VORHANDEN | tr '\n' ' '), bio_hashes vorher=$LIVE_KETTE_BIO"
timeout 600 docker run --rm --network "$NET" -e DATABASE_URL "$PG_IMAGE" \
  sh -c 'exec pg_dump "$DATABASE_URL" -Fc --no-owner --no-acl "$@"' sh "${T_ARGS[@]}" \
  > "$WORK/kette-bio.dump"

# --- 2b. Proof-Server (eigener Postgres) --------------------------------------
docker inspect proof-postgres >/dev/null 2>&1 || { echo "::error::Container proof-postgres fehlt auf dieser Box. Abbruch."; exit 1; }
PROOF_DB="$(docker inspect proof-postgres --format '{{range .Config.Env}}{{println .}}{{end}}' | grep -E '^POSTGRES_DB=' | head -1 | cut -d= -f2- || true)"
PROOF_DB="${PROOF_DB:-aequitas_proof}"
case "$PROOF_DB" in *[!a-z0-9_]*) echo "::error::Unerwarteter Datenbankname in proof-postgres. Abbruch."; exit 1 ;; esac
# Im Container ueber den lokalen Socket (trust) -- kein Passwort noetig.
LIVE_PROOF_BIO="$(docker exec proof-postgres psql -U postgres -d "$PROOF_DB" -t -A -c "SELECT count(*) FROM bio_hashes")"
case "$LIVE_PROOF_BIO" in ''|*[!0-9]*) echo "::error::Zaehlung bio_hashes (Proof) unlesbar. Abbruch."; exit 1 ;; esac
echo "proof: Datenbank $PROOF_DB, bio_hashes vorher=$LIVE_PROOF_BIO"
timeout 600 docker exec proof-postgres pg_dump -U postgres -d "$PROOF_DB" -Fc --no-owner --no-acl \
  > "$WORK/proof.dump"

for d in kette-bio proof; do
  S="$(stat -c %s "$WORK/$d.dump")"
  if [ "$S" -gt "$MAX_PG_BYTES" ]; then
    echo "::error::$d.dump ist $S Bytes (> 300 MB). Abbruch."; exit 1
  fi
  echo "$d.dump: $S Bytes"
done

# --- 2c/2d. SQLite: Coordinator und Vergleichsdienst --------------------------
# Datenverzeichnis aus dem laufenden Container (Bind-Mount nach /data).
datenpfad() {
  docker inspect "$1" --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Source}}{{end}}{{end}}' 2>/dev/null || true
}
sqlite_sichern() { # quelle ziel
  timeout 120 python3 - "$1" "$2" <<'PY'
import sqlite3, sys
src, dst = sys.argv[1], sys.argv[2]
q = sqlite3.connect(f"file:{src}?mode=ro", uri=True, timeout=30)
z = sqlite3.connect(dst)
q.backup(z)  # Online-Backup-API: konsistent auch bei laufenden Schreibern
q.close()
erg = z.execute("PRAGMA integrity_check").fetchall()
if erg != [("ok",)]:
    print(f"integrity_check: {erg[:3]}")
    sys.exit(1)
zeilen = []
for (t,) in z.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name"):
    n = z.execute(f'SELECT count(*) FROM "{t}"').fetchone()[0]
    zeilen.append(f"{t}={n}")
z.close()
print(" ".join(zeilen) or "(keine Tabellen)")
PY
}
SQLITE_ZUSAMMENFASSUNG=""
for paar in aequitas-coordinator:coordinator:/root/coordinator-daten aequitas-matching:matching:/root/aequitas-matching-data; do
  C="${paar%%:*}"; rest="${paar#*:}"; UNTER="${rest%%:*}"; VORGABE="${rest#*:}"
  if ! docker inspect "$C" >/dev/null 2>&1; then
    echo "::warning::Container $C laeuft auf dieser Box nicht -- $UNTER uebersprungen."
    SQLITE_ZUSAMMENFASSUNG="${SQLITE_ZUSAMMENFASSUNG}${UNTER}=fehlt "
    continue
  fi
  DIR="$(datenpfad "$C")"; DIR="${DIR:-$VORGABE}"
  [ -d "$DIR" ] || { echo "::error::$C: Datenverzeichnis $DIR fehlt. Abbruch."; exit 1; }
  N=0
  for f in "$DIR"/*.db; do
    [ -f "$f" ] || continue
    name="$(basename "$f")"
    echo -n "$UNTER/$name: "
    sqlite_sichern "$f" "$WORK/$UNTER/$name" || { echo "::error::$UNTER/$name: Kopie nicht pruefbar. Abbruch."; exit 1; }
    N=$((N + 1))
  done
  [ "$N" -ge 1 ] || { echo "::error::$C laeuft, aber in $DIR liegt keine .db -- falsches Verzeichnis? Abbruch."; exit 1; }
  SQLITE_ZUSAMMENFASSUNG="${SQLITE_ZUSAMMENFASSUNG}${UNTER}=${N} "
done

# --- 3. Postgres-Dumps zurueckspielen (Wegwerf-Postgres, kein Netz, tmpfs) ----
echo "=== Wiederherstellungspruefung ==="
docker run -d --rm --name "$PRUEF" --network none --memory 1500m \
  -e POSTGRES_HOST_AUTH_METHOD=trust \
  --tmpfs /var/lib/postgresql/data:rw,size=1g \
  -v "$WORK:/in:ro" "$PG_IMAGE" >/dev/null
# Ueber TCP pruefen: der Init-Server des Images lauscht nur am Socket.
bereit=0
for i in $(seq 1 60); do
  if docker exec "$PRUEF" pg_isready -h 127.0.0.1 -U postgres -q 2>/dev/null; then bereit=1; break; fi
  sleep 1
done
[ "$bereit" = 1 ] || { echo "::error::Pruef-Postgres startet nicht. Abbruch."; exit 1; }
pq() { docker exec "$PRUEF" psql -h 127.0.0.1 -U postgres -d "$1" -t -A -v ON_ERROR_STOP=1 -c "$2"; }
for d in kette-bio proof; do
  db="${d//-/_}"
  pq postgres "CREATE DATABASE $db" >/dev/null
  timeout 300 docker exec "$PRUEF" pg_restore -h 127.0.0.1 -U postgres --no-owner --no-acl --exit-on-error \
    -d "$db" "/in/$d.dump" || { echo "::error::$d.dump laesst sich nicht zurueckspielen. Abbruch."; exit 1; }
done
R_KETTE_BIO="$(pq kette_bio "SELECT count(*) FROM bio_hashes")"
R_PROOF_BIO="$(pq proof "SELECT count(*) FROM bio_hashes")"
case "${R_KETTE_BIO}x${R_PROOF_BIO}" in
  *[!0-9x]*|x*|*x) echo "::error::Zaehlung nach Wiederherstellung unlesbar. Abbruch."; exit 1 ;;
esac
# bio_hashes waechst nur (Loeschen nur mit Sicherungstabelle, bio_store.js).
if [ "$R_KETTE_BIO" -lt "$LIVE_KETTE_BIO" ] || [ "$R_PROOF_BIO" -lt "$LIVE_PROOF_BIO" ]; then
  echo "::error::Wiederhergestellt kette=$R_KETTE_BIO proof=$R_PROOF_BIO, vorher kette=$LIVE_KETTE_BIO proof=$LIVE_PROOF_BIO -- unvollstaendig. Abbruch."
  exit 1
fi
KETTE_ZEILEN=""
for t in $VORHANDEN; do
  echo "$(pq kette_bio "SELECT 1 FROM pg_tables WHERE schemaname='public' AND tablename='$t'")" | grep -qx 1 \
    || { echo "::error::Tabelle $t fehlt nach der Wiederherstellung. Abbruch."; exit 1; }
  KETTE_ZEILEN="${KETTE_ZEILEN}${t}=$(pq kette_bio "SELECT count(*) FROM public.$t") "
done
PROOF_TABELLEN="$(pq proof "SELECT count(*) FROM pg_tables WHERE schemaname='public'")"
docker rm -f "$PRUEF" >/dev/null 2>&1 || true
echo "wiederhergestellt: kette ${KETTE_ZEILEN}| proof bio_hashes=$R_PROOF_BIO tabellen=$PROOF_TABELLEN"

# --- 4. Verschluesseln auf der Box, Klartext loeschen -------------------------
KLAR="$(du -sb "$WORK" | awk '{print $1}')"
if [ "$KLAR" -gt "$MAX_KLARTEXT" ]; then
  echo "::error::Klartext $KLAR Bytes (> 1 GB). Abbruch."; exit 1
fi
printf '%s\n' "erstellt=$STAMP" "kette_bio_hashes=$R_KETTE_BIO" "proof_bio_hashes=$R_PROOF_BIO" \
  "kette_tabellen=$KETTE_ZEILEN" "sqlite=$SQLITE_ZUSAMMENFASSUNG" > "$WORK/INHALT.txt"
timeout 600 tar -C "$WORK" -cf - . | timeout 600 age -r "$R" -o "$ZIEL.tmp"
rm -rf "$WORK"
[ ! -e "$WORK" ] || { echo "::error::Klartext liess sich nicht loeschen. Abbruch."; exit 1; }

# Die Datei muss age-verschluesselt sein -- nie ein Klartext-tar.
[ "$(head -c 21 "$ZIEL.tmp")" = "age-encryption.org/v1" ] || { echo "::error::Ergebnis ist keine age-Datei. Abbruch."; exit 1; }
[ "$(head -c 4096 "$ZIEL.tmp" | grep -ac '^-> X25519 ')" = 1 ] || { echo "::error::age-Kopf hat nicht genau einen X25519-Empfaenger. Abbruch."; exit 1; }
GROESSE="$(stat -c %s "$ZIEL.tmp")"
if [ "$GROESSE" -gt "$MAX_AGE_BYTES" ]; then
  echo "::error::.age-Datei $GROESSE Bytes (> 500 MB). Abbruch."; exit 1
fi
mv "$ZIEL.tmp" "$ZIEL" && chmod 600 "$ZIEL"
SHA="$(sha256sum "$ZIEL" | awk '{print $1}')"

printf '%s\n' \
  "erstellt=$STAMP" \
  "kette_bio_hashes=$R_KETTE_BIO" \
  "proof_bio_hashes=$R_PROOF_BIO" \
  "proof_tabellen=$PROOF_TABELLEN" \
  "sqlite=$SQLITE_ZUSAMMENFASSUNG" \
  "klartext_bytes=$KLAR" \
  "age_bytes=$GROESSE" \
  "age_sha256=$SHA" > "$SUMMARY"
chmod 600 "$SUMMARY"
cat "$SUMMARY"

# Verschluesselte Kopien auf der Box: die letzten 7 (ohne Schluessel wertlos).
mkdir -p "$BASIS/archiv"
cp "$ZIEL" "$BASIS/archiv/bio-$STAMP.tar.age"
ls -1t "$BASIS"/archiv/bio-*.tar.age 2>/dev/null | tail -n +8 | xargs -r rm -f
echo "verschluesselt: $ZIEL ($GROESSE Bytes), Klartext geloescht"
