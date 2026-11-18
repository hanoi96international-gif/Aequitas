#!/usr/bin/env bash
# Quantengeld S auf diesem Validator: den eigenen Abschnitt eines Tokens selbst
# praeparieren (forschung/quantengeld/DEZENTRAL.md 13). Freiwillig; ohne laeuft
# der Knoten wie bisher. Anleitung: docs/VALIDATOR_EINRICHTEN.md "Quantengeld".
#
# Was das Skript tut (erneut aufrufen ist sicher, nichts wird ueberschrieben):
#   1. legt das Messverzeichnis an (Standard /srv/aequitas-quanten): 0700, im
#      Besitz des Knotenbenutzers (10001). Nur der Knoten und die Messung
#      lesen und schreiben dort.
#   2. legt eine Python-Umgebung mit fest gepinnten Paketen an (pip
#      --require-hashes, forschung/quantengeld/hardware-anforderungen.txt) und
#      kopiert die Messskripte daneben. Aendern kann beides nur root.
#   3. fragt EINMAL nach dem API-Schluessel DEINES IBM-Quantum-Kontos und legt
#      ihn nur fuer root lesbar ab (wie die .env). Er erscheint nie im Log.
#   4. prueft alles ohne IBM: quanten-messen.sh --probe.
#
# Danach den Knoten mit der Zusatzdatei starten:
#   docker compose -f docker-compose.yml -f docker-compose.quanten.yml up -d --build
set -euo pipefail
cd "$(dirname "$0")"

QS_VERZEICHNIS="${QS_VERZEICHNIS:-/srv/aequitas-quanten}"
QS_OPT="${QS_OPT:-/opt/aequitas-quanten}"
QS_SCHLUESSEL="${QS_SCHLUESSEL:-/etc/aequitas-quanten/ibm-token}"
QS_UID=10001
QUELLE="$(cd ../../forschung/quantengeld && pwd)"

rot() { printf '\033[31m%s\033[0m\n' "$*" >&2; }
gruen() { printf '\033[32m%s\033[0m\n' "$*"; }

[ "$(id -u)" = 0 ] || { rot "ABBRUCH: als root starten"; exit 2; }
command -v setpriv >/dev/null || { rot "ABBRUCH: setpriv fehlt (Paket util-linux)"; exit 2; }
python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)' \
  || { rot "ABBRUCH: Python 3.10 oder neuer noetig"; exit 2; }
if ! python3 -c 'import ensurepip, venv' 2>/dev/null; then
  apt-get install -y python3-venv >/dev/null
fi

# 1. Messverzeichnis: kein Verweis, nur der Knotenbenutzer.
if [ -L "$QS_VERZEICHNIS" ]; then rot "ABBRUCH: $QS_VERZEICHNIS ist ein Verweis"; exit 2; fi
install -d -m 0700 -o "$QS_UID" -g "$QS_UID" "$QS_VERZEICHNIS"
gruen "Messverzeichnis $QS_VERZEICHNIS (0700, Benutzer $QS_UID)"

# 2. Python-Umgebung und Messskripte (root gehoert alles, der Knotenbenutzer
#    darf nur lesen und ausfuehren; sein HOME ist ein eigenes Verzeichnis).
install -d -m 0755 -o root -g root "$QS_OPT" "$QS_OPT/quantengeld"
install -d -m 0700 -o "$QS_UID" -g "$QS_UID" "$QS_OPT/home"
[ -x "$QS_OPT/venv/bin/python" ] || python3 -m venv "$QS_OPT/venv"
PIP_ROOT_USER_ACTION=ignore PIP_DISABLE_PIP_VERSION_CHECK=1 \
  "$QS_OPT/venv/bin/pip" install --quiet --require-hashes --no-deps -r "$QUELLE/hardware-anforderungen.txt"
install -m 0644 -o root -g root "$QUELLE"/*.py "$QS_OPT/quantengeld/"
gruen "Python-Umgebung $QS_OPT (Pakete mit festen Hashes)"

# 3. API-Schluessel des eigenen IBM-Quantum-Kontos, nur fuer root.
if [ -s "$QS_SCHLUESSEL" ]; then
  gruen "IBM-Schluessel liegt schon in $QS_SCHLUESSEL -- bleibt, wie er ist."
else
  install -d -m 0700 -o root -g root "$(dirname "$QS_SCHLUESSEL")"
  t="${IBM_QUANTUM_TOKEN:-}"
  if [ -z "$t" ]; then
    read -rsp "API-Schluessel deines IBM-Quantum-Kontos (wird nicht angezeigt): " t
    echo
  fi
  [ -n "$t" ] || { rot "ABBRUCH: kein Schluessel"; exit 2; }
  ( umask 077; printf '%s' "$t" > "$QS_SCHLUESSEL" )
  unset t
  gruen "IBM-Schluessel abgelegt (nur root kann ihn lesen)"
fi

# 4. Probe ohne IBM.
QS_VERZEICHNIS="$QS_VERZEICHNIS" QS_OPT="$QS_OPT" QS_SCHLUESSEL="$QS_SCHLUESSEL" bash ./quanten-messen.sh --probe
gruen "Fertig. Knoten neu starten:"
echo "  docker compose -f docker-compose.yml -f docker-compose.quanten.yml up -d --build"
