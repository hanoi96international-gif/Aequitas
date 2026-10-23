#!/usr/bin/env bash
# Prueft die Angaben fuer Impressum und Datenschutz und gibt sie als
# LEGAL_X=WERT-Zeilen auf stdout aus. Laeuft auf dem Runner
# (.github/workflows/rechtstexte-setzen.yml), die Werte kommen als
# Umgebungsvariablen.
#
# WARUM EINE ERLAUBTE ZEICHENLISTE (23.10.2026)
#
# Die Zeilen landen in deploy/validator/.env, und docker compose liest diese
# Datei mit seinem dotenv-Parser: nach einem schliessenden ' liest er auf
# derselben Zeile weiter (LEGAL_ORT='a' AUTHORIZED_VALIDATORS=x setzt einen
# zweiten Schluessel), ${VAR} wird aus frueheren Zeilen eingesetzt
# (LEGAL_NAME=${NODE_KEY} stuende im oeffentlichen Impressum), # beginnt
# einen Kommentar. Zeichen still zu entfernen hiesse, einen Wert zu
# veroeffentlichen, den niemand so eingegeben hat. Darum: nur Zeichen, die
# in Namen, Adressen, E-Mail und Behoerdennamen vorkommen -- alles andere
# bricht den Lauf ab.
set -euo pipefail
export LC_ALL=C.UTF-8

PFLICHT="LEGAL_NAME LEGAL_STRASSE LEGAL_ORT LEGAL_LAND LEGAL_EMAIL LEGAL_VERANTWORTLICH LEGAL_AUFSICHTSBEHOERDE"
OPTIONAL="LEGAL_TELEFON LEGAL_VERTRETEN_DURCH LEGAL_REGISTERGERICHT LEGAL_REGISTERNUMMER LEGAL_USTID"
# Buchstaben (auch Umlaute und andere Schriften), Ziffern, Leerzeichen und
# . , - / @ + & ( ) : _ -- am Anfang ein Buchstabe, eine Ziffer, + oder (
# (Telefonnummern), am Ende kein Leerzeichen.
ERLAUBT='^[[:alnum:]+(]([[:alnum:] .,/@+&():_-]*[[:alnum:].)])?$'

fehler() { echo "::error::$1" >&2; exit 1; }

# Optionale Angaben aus EINEM Feld (workflow_dispatch erlaubt nur zehn Eingaben).
IFS=';' read -ra OPT <<< "${LEGAL_OPTIONAL:-}"
for paar in "${OPT[@]}"; do
  [ -n "$paar" ] || continue
  key="${paar%%=*}"; val="${paar#*=}"
  [ "$paar" != "$key" ] || fehler "optionale Angabe ohne '=': $key"
  case " $OPTIONAL " in
    *" $key "*) export "$key=$val" ;;
    *) fehler "unbekannte optionale Angabe: $key" ;;
  esac
done

# Erst alles pruefen, dann ausgeben: bricht eine Angabe ab, steht auch von
# den gueltigen nichts auf stdout.
aus=""
for k in $PFLICHT $OPTIONAL; do
  v="${!k:-}"
  if [ -z "$v" ]; then
    case " $PFLICHT " in *" $k "*) fehler "$k fehlt" ;; esac
    continue
  fi
  [ "${#v}" -le 200 ] || fehler "$k ist laenger als 200 Zeichen"
  [[ "$v" =~ $ERLAUBT ]] || fehler "$k enthaelt ein nicht erlaubtes Zeichen (erlaubt: Buchstaben, Ziffern, Leerzeichen und . , - / @ + & ( ) : _; kein Leerzeichen am Anfang oder Ende)"
  aus+="$k=$v"$'\n'
done
printf '%s' "$aus"
