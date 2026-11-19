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
#   5. nur mit --automatisch: ein systemd-Timer misst jede Minute, was
#      Ausgeber beim Knoten vorgemerkt haben (quanten-messen.sh
#      --warteschlange), hoechstens QS_JE_TAG (Standard 8) Abschnitte am Tag.
#      Ohne --automatisch misst du von Hand (bash quanten-messen.sh BASEN
#      oder bash quanten-messen.sh --warteschlange).
#
# Danach den Knoten mit der Zusatzdatei starten:
#   docker compose -f docker-compose.yml -f docker-compose.quanten.yml up -d --build
set -euo pipefail
cd "$(dirname "$0")"

QS_VERZEICHNIS="${QS_VERZEICHNIS:-/srv/aequitas-quanten}"
QS_OPT="${QS_OPT:-/opt/aequitas-quanten}"
QS_SCHLUESSEL="${QS_SCHLUESSEL:-/etc/aequitas-quanten/ibm-token}"
QS_JE_TAG="${QS_JE_TAG:-8}"
QS_SYSTEMD="${QS_SYSTEMD:-/etc/systemd/system}"
QS_UID=10001
QUELLE="$(cd ../../forschung/quantengeld && pwd)"
AUTOMATISCH=0
for a in "$@"; do
  case "$a" in
    --automatisch) AUTOMATISCH=1 ;;
    *) printf 'ABBRUCH: unbekanntes Argument %s (erlaubt: --automatisch)\n' "$a" >&2; exit 2 ;;
  esac
done

rot() { printf '\033[31m%s\033[0m\n' "$*" >&2; }
gruen() { printf '\033[32m%s\033[0m\n' "$*"; }

[ "$(id -u)" = 0 ] || { rot "ABBRUCH: als root starten"; exit 2; }
command -v setpriv >/dev/null || { rot "ABBRUCH: setpriv fehlt (Paket util-linux)"; exit 2; }
python3 -c 'import sys; sys.exit(0 if sys.version_info >= (3, 10) else 1)' \
  || { rot "ABBRUCH: Python 3.10 oder neuer noetig"; exit 2; }
if [ "$AUTOMATISCH" = 1 ]; then
  [[ "$QS_JE_TAG" =~ ^[0-9]{1,2}$ ]] && [ "$QS_JE_TAG" -ge 1 ] && [ "$QS_JE_TAG" -le 64 ] \
    || { rot "ABBRUCH: QS_JE_TAG 1 bis 64"; exit 2; }
  command -v systemctl >/dev/null || { rot "ABBRUCH: kein systemd -- ohne --automatisch einrichten und von Hand messen"; exit 2; }
  [ -d "$QS_SYSTEMD" ] && [ ! -L "$QS_SYSTEMD" ] || { rot "ABBRUCH: $QS_SYSTEMD ist kein Verzeichnis"; exit 2; }
fi
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
# Eine Kopie des Messskripts, die nur root aendern kann (der Timer ruft sie auf).
install -m 0755 -o root -g root ./quanten-messen.sh "$QS_OPT/quanten-messen.sh"
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
# 5. Timer (nur mit --automatisch).
if [ "$AUTOMATISCH" = 1 ]; then
  printf '%s\n' \
    "[Unit]" \
    "Description=Aequitas Quantengeld: vorgemerkte Abschnitte messen (hoechstens $QS_JE_TAG am Tag)" \
    "After=network-online.target" \
    "Wants=network-online.target" \
    "" \
    "[Service]" \
    "Type=oneshot" \
    "Environment=QS_VERZEICHNIS=$QS_VERZEICHNIS QS_OPT=$QS_OPT QS_SCHLUESSEL=$QS_SCHLUESSEL QS_JE_TAG=$QS_JE_TAG" \
    "ExecStart=/bin/bash $QS_OPT/quanten-messen.sh --warteschlange" \
    "TimeoutStartSec=1h" > "$QS_SYSTEMD/aequitas-quanten.service"
  printf '%s\n' \
    "[Unit]" \
    "Description=Aequitas Quantengeld: jede Minute nach Vormerkungen sehen" \
    "" \
    "[Timer]" \
    "OnBootSec=2min" \
    "OnUnitInactiveSec=1min" \
    "" \
    "[Install]" \
    "WantedBy=timers.target" > "$QS_SYSTEMD/aequitas-quanten.timer"
  chmod 0644 "$QS_SYSTEMD/aequitas-quanten.service" "$QS_SYSTEMD/aequitas-quanten.timer"
  systemctl daemon-reload
  systemctl enable --now aequitas-quanten.timer
  gruen "Timer aequitas-quanten.timer laeuft (hoechstens $QS_JE_TAG Messungen am Tag). Protokoll: journalctl -u aequitas-quanten"
fi

gruen "Fertig. Knoten neu starten:"
echo "  docker compose -f docker-compose.yml -f docker-compose.quanten.yml up -d --build"
