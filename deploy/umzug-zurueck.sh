#!/usr/bin/env bash
# Rueckweg des C1-Umzugs, ALTER Server: startet wieder, was vor dem Umzug
# lief, mit der bisherigen Neustart-Regel (/root/.umzug/gestoppt, geschrieben
# von umzug-packen.sh MODUS=ernst). Laeuft ueber c1-umzug.yml phase=zurueck --
# ERST nachdem dort der neue Server angehalten und stumm ist.
set -euo pipefail
L=/root/.umzug/gestoppt
[ -s "$L" ] || { echo "keine Liste $L -- nichts zu starten" >&2; exit 1; }
# Datenbank zuerst, Knoten zuletzt (er braucht sie beim Start).
sortiert="$(grep -E '^[A-Za-z0-9_.-]+ [a-z-]*$' "$L" | awk '{o=2} /postgres/{o=1} /^aequitas-node /{o=3} {print o, $0}' | sort -n | cut -d' ' -f2-)"
[ -n "$sortiert" ] || { echo "$L hat keine gueltige Zeile" >&2; exit 1; }
printf '%s\n' "$sortiert" | while read -r c regel; do
  case "$regel" in no|always|unless-stopped|on-failure) ;; *) regel=unless-stopped ;; esac
  docker update --restart="$regel" "$c" >/dev/null
  docker start "$c" >/dev/null
  echo "gestartet: $c (Neustart: $regel)"
done
