#!/usr/bin/env bash
# Quantengeld S: einen Abschnitt messen (forschung/quantengeld/DEZENTRAL.md 13).
#
#   bash quanten-messen.sh BASEN     BASEN: die Messbasen, die dir die Inhaberin
#                                    fuer DEINEN Abschnitt geschickt hat (Text aus
#                                    0 und 1, hoechstens 16.384 Zeichen)
#   bash quanten-messen.sh --probe   ohne IBM: Selbsttest und eine ideale Messung
#                                    in ein Probeverzeichnis
#
# Gemessen wird mit DEINEM IBM-Konto (Schluessel aus quanten-einrichten.sh), als
# Knotenbenutzer (10001), je Stelle 5 Schuesse mit Mehrheit. Das Ergebnis landet
# als NEUE Datei <sha256(Basen)>.json im Messverzeichnis -- nie ueberschrieben;
# der Knoten verwendet sie genau einmal. Erst danach den Auftrag des Ausgebers
# annehmen. Der Schluessel steht nur in der Umgebung der Messung, nie auf der
# Befehlszeile und nie im Log.
set -euo pipefail

QS_VERZEICHNIS="${QS_VERZEICHNIS:-/srv/aequitas-quanten}"
QS_OPT="${QS_OPT:-/opt/aequitas-quanten}"
QS_SCHLUESSEL="${QS_SCHLUESSEL:-/etc/aequitas-quanten/ibm-token}"
QS_UID=10001
PY="$QS_OPT/venv/bin/python"
SKRIPT="$QS_OPT/quantengeld/hardware_ibm.py"

rot() { printf '\033[31m%s\033[0m\n' "$*" >&2; }
[ "$(id -u)" = 0 ] || { rot "ABBRUCH: als root starten"; exit 2; }
[ -x "$PY" ] && [ -f "$SKRIPT" ] || { rot "ABBRUCH: erst bash quanten-einrichten.sh"; exit 2; }

# Als Knotenbenutzer, mit eigenem HOME; die Umgebung (auch der Schluessel)
# geht an den Prozess, nicht auf die Befehlszeile.
als_knoten() { HOME="$QS_OPT/home" setpriv --reuid "$QS_UID" --regid "$QS_UID" --clear-groups -- "$@"; }

if [ "${1:-}" = "--probe" ]; then
  als_knoten "$PY" -I "$SKRIPT" --selbsttest
  # Probeverzeichnis unter $QS_OPT (gehoert root), nicht in $QS_OPT/home:
  # dort koennte der Knotenbenutzer es zwischen mktemp und chown gegen einen
  # Verweis tauschen, und chown folgt Verweisen.
  probe="$(mktemp -d "$QS_OPT/probe.XXXXXX")"
  chown "$QS_UID:$QS_UID" "$probe"
  trap 'rm -rf -- "$probe"' EXIT
  als_knoten "$PY" -I "$SKRIPT" --ideal --s-abschnitt "$probe" --basen 0110100101101001 | tail -1
  n="$(find "$probe" -maxdepth 1 -name '*.json' | wc -l)"
  [ "$n" = 1 ] || { rot "PROBE GESCHEITERT: $n Messdateien statt 1"; exit 1; }
  [ -d "$QS_VERZEICHNIS" ] && [ ! -L "$QS_VERZEICHNIS" ] \
    && [ "$(stat -c '%u %a' "$QS_VERZEICHNIS")" = "$QS_UID 700" ] \
    || { rot "PROBE GESCHEITERT: $QS_VERZEICHNIS fehlt oder hat andere Rechte als 0700/$QS_UID"; exit 1; }
  echo "Probe bestanden: Messung als Knotenbenutzer moeglich, Messverzeichnis richtig."
  exit 0
fi

BASEN="${1:-}"
# Laenge zuerst und ohne Wiederholungszaehler im Muster: ^[01]{1,16384}$
# wuerde bash bei jedem Aufruf zu einem riesigen Automaten uebersetzen.
[ "${#BASEN}" -ge 1 ] && [ "${#BASEN}" -le 16384 ] && [[ "$BASEN" =~ ^[01]+$ ]] \
  || { rot "ABBRUCH: BASEN nur 0 und 1, 1 bis 16384 Zeichen"; exit 2; }
[ $# -eq 1 ] || { rot "ABBRUCH: genau ein Argument"; exit 2; }
[ -s "$QS_SCHLUESSEL" ] || { rot "ABBRUCH: kein IBM-Schluessel ($QS_SCHLUESSEL) -- erst bash quanten-einrichten.sh"; exit 2; }
IBM_QUANTUM_TOKEN="$(cat "$QS_SCHLUESSEL")"
export IBM_QUANTUM_TOKEN
als_knoten "$PY" -I "$SKRIPT" --ibm --s-abschnitt "$QS_VERZEICHNIS" --basen "$BASEN"
