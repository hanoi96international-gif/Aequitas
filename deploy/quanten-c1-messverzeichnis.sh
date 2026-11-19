#!/usr/bin/env bash
# Messverzeichnis fuer Quantengeld S auf C1 (Abschnitts-Weg mit Stichprobe
# v2, DEZENTRAL.md 16 Schritt 7). Laeuft aus deploy/deploy-c1.sh, vor dem
# Bauen, als root auf C1.
#
# - Verzeichnis QSD (/srv/aequitas-quanten): gehoert dem Knotenbenutzer
#   (10001), 0700, kein Verweis. Darin legt deploy/quanten-s-ausgabe-abschnitt.sh
#   je Ausgabe genau eine Messung ab; der Dienst verbraucht sie einmal.
# - docker-compose.override.yml im Verzeichnis des Knotens: eine Kopie von
#   docker-compose.quanten.yml (AEQUITAS_QS_MESSQUELLE=/data/quanten und das
#   Verzeichnis als /data/quanten). Docker Compose liest diese Datei bei JEDEM
#   Aufruf in diesem Verzeichnis von selbst -- auch bei Ruecknahme und
#   Neustarts aus anderen Skripten. So hat der Knoten die Messquelle immer
#   oder nie, nicht je nach Skript.
#
# Fail-closed: Ein Verweis, ein fremder Besitzer, der sich nicht setzen
# laesst, eine fremde override-Datei (nicht von hier) oder ein abweichendes
# QS_VERZEICHNIS in .env halten den Deploy an, statt etwas zu ueberschreiben.
set -euo pipefail
QSD="${QSD:-/srv/aequitas-quanten}"
QS_UID="${QS_UID-10001}"
VAL="${VAL:-/root/Aequitas/deploy/validator}"
MARKE='# Angelegt von deploy/quanten-c1-messverzeichnis.sh aus docker-compose.quanten.yml -- nicht von Hand aendern.'
halt() { echo "ABBRUCH: $*" >&2; exit 1; }

[[ "$QS_UID" =~ ^[1-9][0-9]{0,9}$ ]] || halt "QS_UID"
cd "$VAL"
[ -f .env ] || halt "$VAL/.env fehlt"
[ -f docker-compose.quanten.yml ] && [ ! -L docker-compose.quanten.yml ] || halt "docker-compose.quanten.yml fehlt"
# docker-compose.quanten.yml bindet ${QS_VERZEICHNIS:-/srv/aequitas-quanten}
# ein; ein anderer Wert in .env zeigte auf ein Verzeichnis, das hier niemand
# eingerichtet hat.
if grep -q '^QS_VERZEICHNIS=' .env; then
  grep -qx "QS_VERZEICHNIS=$QSD" .env || halt "QS_VERZEICHNIS in .env zeigt nicht auf $QSD"
fi

[ ! -L "$QSD" ] || halt "$QSD ist ein Verweis"
[ -e "$QSD" ] || mkdir -m 700 -- "$QSD"
[ -d "$QSD" ] && [ ! -L "$QSD" ] || halt "$QSD ist kein Verzeichnis"
chown "$QS_UID:$QS_UID" -- "$QSD"
chmod 700 -- "$QSD"
[ "$(stat -c '%u:%g %a' "$QSD")" = "$QS_UID:$QS_UID 700" ] || halt "$QSD hat nicht $QS_UID:$QS_UID 0700"

O=docker-compose.override.yml
if [ -e "$O" ] || [ -L "$O" ]; then
  [ -f "$O" ] && [ ! -L "$O" ] && [ "$(head -n 1 "$O")" = "$MARKE" ] \
    || halt "$VAL/$O ist nicht von diesem Skript -- von Hand pruefen, nichts ueberschrieben"
fi
{ printf '%s\n' "$MARKE"; cat docker-compose.quanten.yml; } > "$O.neu"
chmod 644 -- "$O.neu"
mv -f -- "$O.neu" "$O"
echo "Messverzeichnis $QSD ($QS_UID, 0700) und $O aus docker-compose.quanten.yml"
