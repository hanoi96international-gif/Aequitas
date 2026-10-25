#!/usr/bin/env bash
# Test fuer die Schluessel-Reparatur, mit der jeder Server-Workflow
# CONTABO_SSH_KEY (und die anderen SSH-Secrets) in eine Datei schreibt.
#
# WARUM (25.10.2026). Am Handy eingefuegt, verliert ein Secret seine
# Zeilenumbrueche. Ein OpenSSH-Schluessel braucht sie aber an genau zwei
# Stellen (nach -----BEGIN und vor -----END); sonst "Load key: error in
# libcrypto" oder "invalid format". Die Reparatur setzt Kopf, Rumpf
# (ohne jedes Leerzeichen, neu umbrochen) und Fuss wieder zusammen. Sie
# erfindet nichts: was kein vollstaendiger Schluessel war, bleibt unlesbar
# und die Anmeldung scheitert wie vorher (fail-closed).
#
# Geprueft wird die Zeile, wie sie in den Workflows steht:
# scripts/lint_ssh_schluessel.py liest sie aus dieser Datei (REPARATUR).
set -uo pipefail
cd "$(dirname "$0")/.."
command -v ssh-keygen >/dev/null || { echo "FEHLER: ssh-keygen fehlt"; exit 1; }
command -v perl >/dev/null || { echo "FEHLER: perl fehlt"; exit 1; }
REPARATUR="$(python3 scripts/lint_ssh_schluessel.py --reparatur)"
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
fehler=0

ssh-keygen -q -t ed25519 -N '' -C test -f "$T/echt"
ECHT="$(cat "$T/echt")"

# Genau so, wie die Workflows es tun (eval: die Zeile ist ein Stueck Shell).
schreiben() { KEY="$1" bash -c "set -euo pipefail; $REPARATUR > \"\$1\"" _ "$T/ziel"; }

lesen() { # Name, erwartet (0 = lesbar, 1 = unlesbar), Wert
  rm -f "$T/ziel"
  schreiben "$3" || { echo "FEHLER: $1: Reparatur brach ab"; fehler=1; return; }
  chmod 600 "$T/ziel"
  if ssh-keygen -y -f "$T/ziel" >/dev/null 2>&1; then
    [ "$2" = 0 ] || { echo "FEHLER: $1 wurde lesbar"; fehler=1; }
    [ "$2" != 0 ] || [ "$(ssh-keygen -y -f "$T/ziel" | awk '{print $2}')" = "$(awk '{print $2}' "$T/echt.pub")" ] \
      || { echo "FEHLER: $1 ergibt einen anderen Schluessel"; fehler=1; }
  else
    [ "$2" = 1 ] || { echo "FEHLER: $1 nicht lesbar"; fehler=1; }
  fi
}

# Lesbar
lesen "Datei wie von ssh-keygen" 0 "$ECHT"
lesen "Umbrueche als Leerzeichen" 0 "$(printf '%s' "$ECHT" | tr '\n' ' ')"
lesen "alles auf einer Zeile" 0 "$(printf '%s' "$ECHT" | tr -d '\n')"
lesen "Windows-Zeilenenden" 0 "$(printf '%s\n' "$ECHT" | sed 's/$/\r/')"
lesen "Leerzeichen davor und danach" 0 "  $ECHT  "
lesen "Tabs statt Umbrueche" 0 "$(printf '%s' "$ECHT" | tr '\n' '\t')"
lesen "eine base64-Zeile" 0 "$(base64 -w0 "$T/echt")"
lesen "base64 mit Umbruechen" 0 "$(base64 "$T/echt")"
lesen "base64 mit Leerzeichen" 0 "$(base64 "$T/echt" | tr '\n' ' ')"

# Unlesbar -- die Reparatur macht daraus keinen Schluessel
lesen "leer" 1 ""
lesen "Unsinn" 1 "hallo welt"
lesen "Rumpf abgeschnitten" 1 "$(printf '%s\n' "$ECHT" | sed '3d')"
lesen "Fuss fehlt" 1 "$(printf '%s\n' "$ECHT" | head -n -1)"
lesen "oeffentlicher statt privater Schluessel" 1 "$(cat "$T/echt.pub")"
lesen "Gedankenstriche statt Bindestriche (iOS)" 0 "$(printf '%s' "$ECHT" | sed 's/--/—/g' | tr '\n' ' ')"
lesen "Halbgeviertstriche statt Bindestriche" 0 "$(printf '%s\n' "$ECHT" | sed 's/--/–/g')"
lesen "Gedankenstrich im Rumpf" 1 "$(printf '%s\n' "$ECHT" | sed '3s/^./—/')"
lesen "base64 von Unsinn" 1 "$(printf 'hallo welt' | base64 -w0)"
lesen "base64 abgeschnitten" 1 "$(base64 -w0 "$T/echt" | cut -c1-200)"
lesen "base64 mit Fremdzeichen" 1 "$(base64 -w0 "$T/echt" | sed 's/^/;id;/')"

# Ein richtig eingefuegter Schluessel bleibt byte-gleich.
schreiben "$ECHT"
cmp -s "$T/ziel" "$T/echt" || { echo "FEHLER: richtiger Schluessel wird veraendert"; fehler=1; }

# Kein Schluesselinhalt in der Ausgabe der Reparatur ausser in der Datei.
aus="$(KEY="$(printf '%s' "$ECHT" | tr '\n' ' ')" bash -c "$REPARATUR > /dev/null" 2>&1)"
[ -z "$aus" ] || { echo "FEHLER: Reparatur schreibt auf stdout/stderr"; fehler=1; }

if [ "$fehler" = 0 ]; then echo "ssh-schluessel-reparatur: 11 Schreibweisen lesbar, 9 Faelle bleiben unlesbar, richtiger Schluessel unveraendert"; fi
exit "$fehler"
