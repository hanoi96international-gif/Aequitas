#!/usr/bin/env bash
# Missbrauchstest fuer deploy/rechtstexte-angaben.sh (siehe dessen Kopf):
# echte Angaben gehen durch, jede Eingabe, die docker compose als zweiten
# Schluessel, als Variable oder als Kommentar lesen wuerde, bricht ab.
set -uo pipefail
cd "$(dirname "$0")/.."
S=deploy/rechtstexte-angaben.sh
fehler=0

gut() {
  env -i PATH="$PATH" LEGAL_NAME="Anna Müller" LEGAL_STRASSE="Hauptstraße 12a" LEGAL_ORT="10115 Berlin" \
    LEGAL_LAND="Deutschland" LEGAL_EMAIL="kontakt+impressum@example.org" LEGAL_VERANTWORTLICH="Anna Müller" \
    LEGAL_AUFSICHTSBEHOERDE="Berliner Beauftragte für Datenschutz und Informationsfreiheit (BlnBDI)" "$@" bash "$S"
}

# Gutfall
aus=$(gut LEGAL_OPTIONAL="LEGAL_TELEFON=+49 30 1234-5678;LEGAL_USTID=DE123456789") || { echo "FEHLER: Gutfall abgelehnt"; fehler=1; }
[ "$(printf '%s\n' "$aus" | grep -c '^LEGAL_')" = 9 ] || { echo "FEHLER: Gutfall ergibt nicht 9 Zeilen:"; echo "$aus"; fehler=1; }
printf '%s\n' "$aus" | grep -qx 'LEGAL_STRASSE=Hauptstraße 12a' || { echo "FEHLER: Wert veraendert"; fehler=1; }

# Angriffe: jeder muss scheitern und darf nichts ausgeben
schlecht() {
  local name="$1"; shift
  if out=$(gut "$@" 2>/dev/null); then
    echo "FEHLER: angenommen: $name"; echo "$out" | sed 's/^/    /'; fehler=1
  elif [ -n "$out" ]; then
    echo "FEHLER: $name gab trotz Abbruch etwas aus"; fehler=1
  fi
}
schlecht "zweiter Schluessel nach '"        LEGAL_ORT="'a' AUTHORIZED_VALIDATORS=evil"
schlecht "Variable \${…}"                  LEGAL_NAME='${NODE_KEY}'
schlecht "Variable \$X"                    LEGAL_NAME='$POSTGRES_PASSWORD'
schlecht "Kommentar #"                     LEGAL_STRASSE="Weg 1 #x"
schlecht "Gleichheitszeichen"              LEGAL_LAND="X=Y"
schlecht "Anfuehrungszeichen"              LEGAL_NAME='A"B'
schlecht "Backtick"                        LEGAL_NAME='A`id`'
schlecht "Backslash"                       LEGAL_NAME='A\nAUTHORIZED_VALIDATORS=x'
schlecht "Zeilenumbruch"                   LEGAL_NAME=$'A\nAUTHORIZED_VALIDATORS=x'
schlecht "Wagenruecklauf"                  LEGAL_NAME=$'A\rB'
schlecht "Leerzeichen am Ende"             LEGAL_NAME="Anna "
schlecht "Leerzeichen am Anfang"           LEGAL_NAME=" Anna"
schlecht "Pflichtfeld leer"                LEGAL_EMAIL=""
schlecht "zu lang"                         LEGAL_NAME="$(printf 'a%.0s' $(seq 1 201))"
schlecht "unbekannte optionale Angabe"     LEGAL_OPTIONAL="AUTHORIZED_VALIDATORS=x"
schlecht "optionale Angabe mit '"          LEGAL_OPTIONAL="LEGAL_TELEFON='1' IS_PRIMARY_NODE=true"
schlecht "optionale Angabe ohne ="         LEGAL_OPTIONAL="LEGAL_TELEFON"

[ "$fehler" = 0 ] && echo "rechtstexte-angaben: Gutfall und 17 Angriffe wie erwartet"
exit "$fehler"
