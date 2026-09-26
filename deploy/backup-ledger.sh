#!/usr/bin/env bash
# Zustands-Backup eines Validators: pg_dump (ohne Bloecke), Wiederherstellung
# in eine Wegwerf-Datenbank pruefen, Kopie fuer die Auslagerung ablegen.
# Laeuft ueber .github/workflows/backup-ledger.yml auf der Box selbst
# (BOX=<name> bash -s < deploy/backup-ledger.sh).
#
# ZWEI DATEIEN. /root/backups/aequitas-<zeit>.dump ist der vollstaendige,
# gepruefte Dump -- er bleibt auf der Box (die letzten 7). Ausgelagert wird
# /root/backup-latest.dump: derselbe Zustand OHNE die Zeilen der Tabellen
# mit biometrischem Bezug. Das Repo ist oeffentlich, und damit jedes
# Artefakt fuer jeden angemeldeten GitHub-Nutzer lesbar (26.09.2026). Die
# MPC-Anteile (mpc_shares) sind je Validator fuer sich Rauschen -- die
# Dumps ZWEIER Validatoren zusammen ergeben die Gesichts-Templates
# (mpc_store.go). bio_hashes und Verwandte ordnen Identitaets-Hashes
# Wallets zu. Nichts davon gehoert in eine oeffentliche Ablage.
set -euo pipefail
BOX="${BOX:?BOX setzen}"

DATABASE_URL="$(grep -E '^DATABASE_URL=' /root/.aequitas.env 2>/dev/null | head -1 | cut -d= -f2- || true)"
if [ -z "$DATABASE_URL" ]; then
  DATABASE_URL="$(docker inspect aequitas-node --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E '^DATABASE_URL=' | head -1 | cut -d= -f2- || true)"
fi
if [ -z "$DATABASE_URL" ]; then
  echo "No DATABASE_URL on this box — cannot back up. Failing loudly rather than reporting success."
  exit 1
fi

# DATABASE_URL's host is a docker DNS name that only resolves on
# aequitas-node's own network — same reason every other diagnostic
# workflow here joins it.
NET="$(docker inspect aequitas-node --format '{{range $n,$v := .NetworkSettings.Networks}}{{$n}}{{end}}')"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="/root/backups"
# Eine Kopie aus einem frueheren Lauf darf nicht als die heutige
# ausgeliefert werden, wenn pg_dump gleich scheitert.
rm -f /root/backup-latest.dump
mkdir -p "$OUT"
DUMP="$OUT/aequitas-${STAMP}.dump"

echo "=== pg_dump (custom format, compressed; Zustand ohne Bloecke) ==="
# -Fc so the restore check can use pg_restore and so the file is
# compressed without a second pass. --no-owner/--no-acl so it can be
# restored into a fresh database that has different role names.
# --exclude-table-data: die Tabellen bleiben im Schema, nur ihre
# Zeilen fehlen -- ein Restore ergibt eine Datenbank, an die sich
# ein Knoten anschliessen und die Bloecke vom Netz holen kann.
# Siehe Kopf der Datei, warum die Bloecke nicht mit hinein gehoeren.
docker run --rm --network "$NET" -v "$OUT:/out" postgres:16-alpine \
  pg_dump "$DATABASE_URL" -Fc --no-owner --no-acl \
    --exclude-table-data=chain_blocks \
    --exclude-table-data=chain_tx_batches \
    --exclude-table-data=chain_tx_block_index \
    --exclude-table-data=evm_tx_receipts \
    --exclude-table-data=pending_txs \
    -f "/out/$(basename "$DUMP")"

ls -lh "$DUMP"
SIZE=$(stat -c %s "$DUMP")
if [ "$SIZE" -lt 100000 ]; then
  echo "Dump is only ${SIZE} bytes — that is not a whole ledger. Failing."
  exit 1
fi
# Regression guard: state-only dumps are <<200MB. A multi-GB file means
# chain_blocks (or similar) leaked back into the dump — the failure mode
# that burned the 06.–12.09.2026 cron runs on the 30-minute SSH timeout.
MAX_BYTES=$((500 * 1024 * 1024))
if [ "$SIZE" -gt "$MAX_BYTES" ]; then
  echo "Dump is ${SIZE} bytes (>500MB) — looks like a full chain dump (exclude missing?). Failing."
  exit 1
fi

# Die Kopie fuer die Auslagerung entsteht HIER, vor der Pruefung.
#
# Vorher stand sie ganz am Ende. Jeder Fehlschlag der Pruefung sprang
# per exit 1 daran vorbei, und damit liefen auch die beiden
# Auslagerungsschritte nicht -- die Sicherung blieb ausschliesslich
# auf derselben Box, die sie schuetzen soll. Genau so ist es sieben
# von sieben Laeufen ergangen.
#
# Eine ungepruefte Kopie ist weniger wert als eine gepruefte, aber
# unendlich viel mehr wert als keine. Der Lauf endet trotzdem rot,
# wenn die Pruefung scheitert: "wir haben eine Kopie" und "wir haben
# bewiesen, dass sie sich zurueckspielen laesst" sind zwei getrennte
# Aussagen, und die zweite darf die erste nicht loeschen.
# Ausgelagert wird eine zweite Fassung ohne biometrische Tabellenzeilen
# (siehe Kopf). Das Schema bleibt vollstaendig.
docker run --rm --network "$NET" -v /root:/root_out postgres:16-alpine \
  pg_dump "$DATABASE_URL" -Fc --no-owner --no-acl \
    --exclude-table-data=chain_blocks \
    --exclude-table-data=chain_tx_batches \
    --exclude-table-data=chain_tx_block_index \
    --exclude-table-data=evm_tx_receipts \
    --exclude-table-data=pending_txs \
    --exclude-table-data=mpc_shares \
    --exclude-table-data=bio_hashes \
    --exclude-table-data=bio_registrations \
    --exclude-table-data=bio_registrations_bio_hash_dedup_backup \
    --exclude-table-data=registration_recovery \
    -f /root_out/backup-latest.dump

echo "=== restore verification (throwaway database, dropped afterwards) ==="
# A dump nobody has restored is a file, not a backup.
ADMIN="$(echo "$DATABASE_URL" | sed 's#/[^/?]*\(?\|$\)#/postgres\1#')"
# KLEINGESCHRIEBEN, und das ist der ganze Fehler gewesen.
#
# STAMP enthaelt ein T und ein Z (20260826T040230Z). CREATE DATABASE
# faltet unquotierte Bezeichner auf Kleinschreibung, also entstand
# ...t040230z -- waehrend der Verbindungs-String den Namen
# buchstabengetreu uebernimmt. pg_restore suchte damit eine Datenbank,
# die es unter diesem Namen nie gab:
#   FATAL: database "aequitas_restore_check_20260826T040230Z" does not exist
# Die Pruefung ist daran seit ihrer Einfuehrung JEDES Mal gescheitert.
VERIFY="$(echo "aequitas_restore_check_${STAMP}" | tr '[:upper:]' '[:lower:]')"
# ALLE liegengebliebenen Pruefkopien abraeumen, nicht nur die eigene.
# Der DROP am Ende dieses Schritts laeuft nur auf dem Glueckspfad;
# stirbt der Job vorher (Zeitlimit, volle Platte beim Restore --
# 11.09.2026), bleibt die Kopie. Am 12.09.2026 lagen so 96 GB auf
# C1 (fuenf Kopien zu je 10-23 GB) und 12 GB auf C2, die Platten
# standen bei 96 und 98 Prozent, und C2 lehnte Ueberweisungen ab.
for ALT in $(docker run --rm --network "$NET" postgres:16-alpine \
    psql "$ADMIN" -t -A -c "SELECT datname FROM pg_database WHERE datname LIKE 'aequitas_restore_check_%';"); do
  echo "   entferne liegengebliebene Pruefkopie $ALT"
  docker run --rm --network "$NET" postgres:16-alpine \
    psql "$ADMIN" -c "DROP DATABASE IF EXISTS ${ALT} WITH (FORCE);" >/dev/null || true
done
docker run --rm --network "$NET" postgres:16-alpine \
  psql "$ADMIN" -c "CREATE DATABASE ${VERIFY};" >/dev/null

VERIFY_URL="$(echo "$DATABASE_URL" | sed "s#/[^/?]*\(?\|$\)#/${VERIFY}\1#")"
set +e
docker run --rm --network "$NET" -v "$OUT:/out" postgres:16-alpine \
  pg_restore --no-owner --no-acl -d "$VERIFY_URL" "/out/$(basename "$DUMP")"
RESTORE_RC=$?
set -e

echo "=== does the restored copy actually hold the ledger? ==="
q() { docker run --rm --network "$NET" postgres:16-alpine psql "$VERIFY_URL" -t -A -c "$1"; }
HUMANS="$(q "SELECT count(*) FROM chain_accounts WHERE is_human = true" || echo 0)"
ACCOUNTS="$(q "SELECT count(*) FROM chain_accounts" || echo 0)"
NULLS="$(q "SELECT count(*) FROM nullifiers" || echo 0)"
SUPPLY="$(q "SELECT COALESCE(sum(balance),0) FROM chain_accounts" || echo 0)"
echo "restored: humans=${HUMANS} accounts=${ACCOUNTS} nullifiers=${NULLS} sum_balance=${SUPPLY} (Bloecke absichtlich nicht enthalten)"

docker run --rm --network "$NET" postgres:16-alpine \
  psql "$ADMIN" -c "DROP DATABASE IF EXISTS ${VERIFY} WITH (FORCE);" >/dev/null || true

if [ "$RESTORE_RC" -ne 0 ]; then
  echo "pg_restore exited ${RESTORE_RC} — this dump is not restorable. Failing."
  exit 1
fi
if [ "${HUMANS:-0}" -lt 1 ] || [ "${ACCOUNTS:-0}" -lt 1 ]; then
  echo "The restored copy has humans=${HUMANS} accounts=${ACCOUNTS} — it restored, but it is empty. Failing."
  exit 1
fi
echo "✓ dump restores cleanly and contains a real ledger"

DUMP_MB=$(( (SIZE + 1024*1024 - 1) / (1024*1024) ))
printf '%s\n' \
  "box=${BOX}" \
  "humans=${HUMANS}" \
  "accounts=${ACCOUNTS}" \
  "nullifiers=${NULLS}" \
  "dump_bytes=${SIZE}" \
  "dump_mb=${DUMP_MB}" \
  > /root/backup-latest.summary
echo "=== backup summary ==="
cat /root/backup-latest.summary

echo "=== retention on the box: keep the last 7, drop older ==="
ls -1t "$OUT"/aequitas-*.dump 2>/dev/null | tail -n +8 | xargs -r rm -f
ls -lh "$OUT"
