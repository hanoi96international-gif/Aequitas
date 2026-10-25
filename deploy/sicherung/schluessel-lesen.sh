#!/usr/bin/env bash
# Schreibt den Sicherungsschluessel aus $KEY nach $1 und prueft ihn gegen den
# oeffentlichen Schluessel $2 (deploy/sicherung/sicherung_ed25519.pub).
#
# WARUM (25.10.2026). Am Handy eingefuegt, verlor das Secret seine
# Zeilenumbrueche; ssh meldete nur "Load key: error in libcrypto". Angenommen
# wird deshalb:
#   - der Schluessel wie ssh-keygen ihn schreibt (-----BEGIN ... -----END),
#     auch mit Leerzeichen statt Umbruechen oder mit CR (Windows);
#   - dieselbe Datei als EINE base64-Zeile (base64 -w0 datei).
# Jede andere Eingabe bricht ab. Danach muss der Schluessel sich lesen lassen
# UND zum oeffentlichen Schluessel im Repo passen -- sonst rot, mit einem
# Satz, was nicht stimmt. Der Wert erscheint nie in der Ausgabe.
set -euo pipefail
umask 077
ZIEL="${1:?Zieldatei angeben}"
PUB="${2:?oeffentlichen Schluessel angeben}"
KOPF="-----BEGIN OPENSSH PRIVATE KEY-----"
FUSS="-----END OPENSSH PRIVATE KEY-----"

K="${KEY:-}"
[ -n "$K" ] || { echo "::error title=SICHERUNG_SSH_KEY fehlt::Secret SICHERUNG_SSH_KEY in der Umgebung sicherung anlegen (docs/SERVER_FREIGABE.md, Abschnitt Sicherung)."; exit 1; }
# Hoechstens 16 KiB: ein ed25519-Schluessel hat rund 400 Bytes.
[ "${#K}" -le 16384 ] || { echo "::error::SICHERUNG_SSH_KEY ist zu lang."; exit 1; }

case "$K" in
  *"$KOPF"*) ;;
  *)
    # Eine base64-Zeile der ganzen Datei.
    if ! K="$(printf '%s' "$K" | tr -d ' \t\r\n' | base64 -d 2>/dev/null)" || [ -z "$K" ]; then
      echo "::error title=SICHERUNG_SSH_KEY unlesbar::Weder ein OpenSSH-Schluessel noch base64 davon."
      exit 1
    fi
    case "$K" in *"$KOPF"*) ;; *) echo "::error title=SICHERUNG_SSH_KEY unlesbar::base64 enthaelt keinen OpenSSH-Schluessel."; exit 1 ;; esac
    ;;
esac

# Rumpf zwischen Kopf und Fuss, ohne jedes Leerzeichen, neu umbrochen.
REST="${K#*"$KOPF"}"
case "$REST" in *"$FUSS"*) ;; *) echo "::error title=SICHERUNG_SSH_KEY unvollstaendig::Die Zeile $FUSS fehlt -- beim Einfuegen abgeschnitten?"; exit 1 ;; esac
RUMPF="$(printf '%s' "${REST%%"$FUSS"*}" | tr -d ' \t\r\n')"
case "$RUMPF" in ''|*[!A-Za-z0-9+/=]*) echo "::error title=SICHERUNG_SSH_KEY unlesbar::Der Rumpf ist kein base64."; exit 1 ;; esac
{ printf '%s\n' "$KOPF"; printf '%s\n' "$RUMPF" | fold -w 70; printf '%s\n' "$FUSS"; } > "$ZIEL"
chmod 600 "$ZIEL"

if ! ABGELEITET="$(ssh-keygen -y -f "$ZIEL" 2>/dev/null)"; then
  rm -f "$ZIEL"
  echo "::error title=SICHERUNG_SSH_KEY unlesbar::ssh-keygen kann den Schluessel nicht lesen -- unvollstaendig eingefuegt?"
  exit 1
fi
IST="$(printf '%s\n' "$ABGELEITET" | awk '{print $1, $2}')"
SOLL="$(awk '{print $1, $2}' "$PUB")"
if [ "$IST" != "$SOLL" ]; then
  rm -f "$ZIEL"
  echo "::error title=SICHERUNG_SSH_KEY passt nicht::Das Secret gehoert nicht zu deploy/sicherung/sicherung_ed25519.pub -- neuen Schluessel einfuegen."
  exit 1
fi
echo "Sicherungsschluessel gelesen, passt zu $(ssh-keygen -lf "$PUB" | awk '{print $2}')"
