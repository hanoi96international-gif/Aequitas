#!/usr/bin/env bash
# Prueft den Inhaltscheck aus deploy/backup-ledger.sh (enthaelt_kontentabelle)
# mit einem nachgebauten docker (Missbrauchs-/Fehlertest nach AGENTS.md):
#
#   1. Der Treffer steht vorn, danach kommt noch viel Ausgabe -- genau der
#      Fall vom 04.11.2026. Die alte Form (pg_restore -l | grep -q) scheitert
#      daran unter pipefail (Gegenprobe), die neue nicht.
#   2. Keine Kontentabelle in der Liste -> 1 (kein Ledger).
#   3. pg_restore -l selbst scheitert -> 2 (Dump unlesbar), nie "gut".
set -euo pipefail
WURZEL=$(cd "$(dirname "$0")/.." && pwd)
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
mkdir -p "$T/fake"
cat > "$T/fake/docker" <<'DOCKER'
#!/bin/bash
case "${FALL:-gut}" in
  gut)    echo "1; 0 0 TABLE DATA public chain_accounts postgres"; sleep 0.2
          for i in $(seq 1 100000); do echo "$i; 0 0 TABLE DATA public x_$i postgres"; done ;;
  ohne)   for i in $(seq 1 2000); do echo "$i; 0 0 TABLE DATA public x_$i postgres"; done ;;
  kaputt) echo "pg_restore: error: input file does not appear to be a valid archive" >&2; exit 1 ;;
esac
DOCKER
chmod +x "$T/fake/docker"

sed -n '/^enthaelt_kontentabelle() {/,/^}/p' "$WURZEL/deploy/backup-ledger.sh" > "$T/funktion.sh"
[ -s "$T/funktion.sh" ] || { echo "FEHLER: enthaelt_kontentabelle nicht in backup-ledger.sh"; exit 1; }
OUT=$T DUMP=$T/x.dump
# shellcheck disable=SC1091
. "$T/funktion.sh"
export PATH="$T/fake:$PATH"

fehler=0
pruefe() { # name erwarteter-rc fall
  local rc=0
  FALL=$3 enthaelt_kontentabelle >/dev/null 2>&1 || rc=$?
  if [ "$rc" != "$2" ]; then echo "FEHLER: $1 (rc=$rc, erwartet $2)"; fehler=1; fi
}
pruefe "Treffer vorn, viel Ausgabe danach -> gefunden" 0 gut
pruefe "keine Kontentabelle -> kein Ledger" 1 ohne
pruefe "pg_restore scheitert -> unlesbar" 2 kaputt

# Gegenprobe: die alte Form scheitert am selben Fall. Sonst zeigte dieser
# Test den Fehler gar nicht.
alt=0
FALL=gut bash -c 'set -o pipefail; docker run x pg_restore -l y | grep -q "TABLE DATA public chain_accounts "' 2>/dev/null || alt=$?
if [ "$alt" = 0 ]; then echo "FEHLER: Gegenprobe -- die alte Form bestand, der Test erzwingt den Wettlauf nicht"; fehler=1; fi

[ "$fehler" = 0 ] && echo "test_backup_ledger_liste: bestanden" || exit 1
