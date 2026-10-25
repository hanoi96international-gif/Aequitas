#!/usr/bin/env bash
# Test fuer deploy/sicherung/schluessel-lesen.sh: der Sicherungsschluessel
# wird so angenommen, wie er am Handy ins Secret gerät (ohne Umbrueche, mit
# CR, als eine base64-Zeile), und alles andere -- abgeschnitten, fremder
# Schluessel, Unsinn -- bricht ab, ohne eine Schluesseldatei zu hinterlassen
# und ohne den Wert auszugeben.
set -uo pipefail
cd "$(dirname "$0")/.."
command -v ssh-keygen >/dev/null || { echo "FEHLER: ssh-keygen fehlt"; exit 1; }
S=deploy/sicherung/schluessel-lesen.sh
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
fehler=0

ssh-keygen -q -t ed25519 -N '' -C test -f "$T/echt"
ssh-keygen -q -t ed25519 -N '' -C fremd -f "$T/fremd"
ECHT="$(cat "$T/echt")"

lesen() { # Name, erwarteter Erfolg (0/1), Wert
  local rc=0 aus
  rm -f "$T/ziel"
  aus="$(env -i PATH="$PATH" KEY="$3" bash "$S" "$T/ziel" "$T/echt.pub" 2>&1)" || rc=$?
  if [ "$2" = 0 ]; then
    [ "$rc" = 0 ] || { echo "FEHLER: $1 abgelehnt: $aus"; fehler=1; return; }
    ssh-keygen -y -f "$T/ziel" >/dev/null 2>&1 || { echo "FEHLER: $1 ergibt keinen lesbaren Schluessel"; fehler=1; }
    [ "$(stat -c %a "$T/ziel")" = 600 ] || { echo "FEHLER: $1 Schluesseldatei nicht 600"; fehler=1; }
  else
    [ "$rc" != 0 ] || { echo "FEHLER: $1 angenommen"; fehler=1; }
    [ ! -e "$T/ziel" ] || { echo "FEHLER: $1 hinterlaesst eine Schluesseldatei"; fehler=1; }
  fi
  # Nie Teile des Schluessels in der Ausgabe.
  RUMPF_ZEILE="$(sed -n 2p "$T/echt")"
  case "$aus" in *"$RUMPF_ZEILE"*) echo "FEHLER: $1 gibt den Schluessel aus"; fehler=1 ;; esac
}

# Angenommen
lesen "Datei wie von ssh-keygen" 0 "$ECHT"
lesen "Umbrueche als Leerzeichen" 0 "$(printf '%s' "$ECHT" | tr '\n' ' ')"
lesen "alles auf einer Zeile" 0 "$(printf '%s' "$ECHT" | tr -d '\n')"
lesen "Windows-Zeilenenden" 0 "$(printf '%s\n' "$ECHT" | sed 's/$/\r/')"
lesen "eine base64-Zeile" 0 "$(base64 -w0 "$T/echt")"
lesen "base64 mit Umbruechen" 0 "$(base64 "$T/echt")"

# Abgelehnt
lesen "leer" 1 ""
lesen "fremder Schluessel" 1 "$(cat "$T/fremd")"
lesen "fremder Schluessel als base64" 1 "$(base64 -w0 "$T/fremd")"
lesen "Fuss abgeschnitten" 1 "$(printf '%s\n' "$ECHT" | head -n -1)"
lesen "Rumpf abgeschnitten" 1 "$(printf '%s\n' "$ECHT" | sed '3d')"
lesen "oeffentlicher statt privater Schluessel" 1 "$(cat "$T/echt.pub")"
lesen "Unsinn" 1 "hallo welt"
lesen "Rumpf mit Fremdzeichen" 1 "$(printf '%s\n' "$ECHT" | sed '2s/^/$(id)/')"
lesen "zu lang" 1 "$(head -c 20000 /dev/zero | tr '\0' A)"

if [ "$fehler" = 0 ]; then echo "schluessel-lesen: 6 Schreibweisen angenommen, 9 Faelle abgelehnt"; fi
exit "$fehler"
