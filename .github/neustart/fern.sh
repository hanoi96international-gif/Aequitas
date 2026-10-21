#!/usr/bin/env bash
# fern.sh HOST UMGEBUNG SKRIPT...: fuehrt die Skripte nacheinander auf HOST aus
# (ssh ... "UMGEBUNG bash -s") und gilt nur als gelungen, wenn bash dort mit 0
# endet UND die letzte Zeile erreicht hat.
#
# Warum (21.10.2026): bash -s liest das Skript von der Standardeingabe. Ein
# Befehl darin, der die Standardeingabe ebenfalls liest (docker exec -i ohne
# eigene Eingabe), schluckte den Rest von leeren.sh. bash sah das Ende der
# Eingabe und meldete 0 -- der Job war gruen, geloescht war nichts. Ein
# verschlucktes oder abgeschnittenes Skript erreicht die Schlussmarke nie.
#
# SSH (Vorgabe ssh) ist nur fuer scripts/test_neustart_fern.sh austauschbar.
set -euo pipefail
[ $# -ge 3 ] || { echo "Aufruf: fern.sh HOST UMGEBUNG SKRIPT..." >&2; exit 2; }
HOST="$1"; UMGEBUNG="$2"; shift 2
SSH="${SSH:-ssh}"
ENDE="NEUSTART-SKRIPT-ENDE-$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
AUS="$(mktemp)"
trap 'rm -f "$AUS"' EXIT

set +e
{ cat "$@"; printf '\necho %s\n' "$ENDE"; } \
  | "$SSH" -i ~/.ssh/k -o BatchMode=yes -o ServerAliveInterval=30 "root@$HOST" "$UMGEBUNG bash -s" \
  | tee "$AUS"
RC=("${PIPESTATUS[@]}")
set -e

[ "${RC[0]}" = 0 ] || { echo "::error::Skripte nicht lesbar ($*)"; exit 1; }
[ "${RC[1]}" = 0 ] || { echo "::error::$* auf $HOST endete mit ${RC[1]}"; exit 1; }
grep -qxF "$ENDE" "$AUS" || {
  echo "::error::$* auf $HOST lief nicht bis zum Ende (Eingabe verschluckt oder abgebrochen) -- als Fehler gewertet"
  exit 1
}
