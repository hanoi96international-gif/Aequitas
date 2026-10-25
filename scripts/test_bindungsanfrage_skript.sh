#!/usr/bin/env bash
# Test fuer den Teil von deploy/validator/einrichten.sh und
# deploy/knoten-binden.sh, der aus /api/validator-selfproof die
# Bindungsanfrage baut (keeper/bindungsanfrage.go).
#
# Seit 25.10.2026 traegt die Anfrage den Nachweis fuer das Validator-Register
# auf der Kette (bindung_zeit, bindung_signatur_knoten), damit die App mit
# einer Bestaetigung auch die Kettenbindung einreicht. Geprueft wird:
#   - beide Skripte bauen die Anfrage mit demselben Programm,
#   - der Kettennachweis wird nur in gueltiger Form uebernommen,
#   - eine Antwort, die nicht zu DIESER Signieradresse gehoert oder keinen
#     gueltigen Schluesselnachweis hat, ergibt keine Anfrage (fail-closed).
set -uo pipefail
cd "$(dirname "$0")/.."
fehler=0

programm() { # Datei -> das python-Programm zwischen python3 -c ' und ' "$ADDR"
  python3 - "$1" <<'PY'
import re, sys
t = open(sys.argv[1]).read()
m = re.findall(r"python3 -c '\n(import json, re, sys\n.*?)\n' \"\$ADDR\"", t, re.S)
if len(m) != 1:
    sys.exit("erwartet genau ein Anfrage-Programm in " + sys.argv[1] + ", gefunden " + str(len(m)))
print(m[0])
PY
}

A="$(programm deploy/validator/einrichten.sh)" || { echo "FEHLER: einrichten.sh"; exit 1; }
B="$(programm deploy/knoten-binden.sh)" || { echo "FEHLER: knoten-binden.sh"; exit 1; }
[ "$A" = "$B" ] || { echo "FEHLER: einrichten.sh und knoten-binden.sh bauen die Anfrage verschieden"; fehler=1; }

ADR=0x1111111111111111111111111111111111111111
WAL=0x2222222222222222222222222222222222222222
SIG="0x$(printf 'a%.0s' $(seq 1 130))"

bauen() { printf '%s' "$1" | python3 -c "$A" "$ADR" "$WAL"; }

pruefe() { # Name, Eingabe, erwartet: "nein" | Python-Ausdruck ueber p
  local aus
  if ! aus="$(bauen "$2")"; then
    [ "$3" = "nein" ] || { echo "FEHLER: $1: keine Anfrage gebaut"; fehler=1; }
    return
  fi
  [ "$3" != "nein" ] || { echo "FEHLER: $1: Anfrage gebaut: $aus"; fehler=1; return; }
  printf '%s' "$aus" | python3 -c "import json,sys; p=json.load(sys.stdin); sys.exit(0 if ($3) else 1)" \
    || { echo "FEHLER: $1: $aus"; fehler=1; }
}

# Gutfall mit und ohne Kettennachweis
pruefe "mit Kette" "{\"signing_address\":\"$ADR\",\"signing_key_signature\":\"$SIG\",\"bindung_zeit\":1792900000,\"bindung_signatur_knoten\":\"$SIG\"}" \
  "p=={'signing_address':'$ADR','wallet':'$WAL','beweis':'$SIG','bindung_zeit':1792900000,'bindung_signatur_knoten':'$SIG'}"
pruefe "ohne Kette (vor dem Stichtag)" "{\"signing_address\":\"$ADR\",\"signing_key_signature\":\"$SIG\"}" \
  "p=={'signing_address':'$ADR','wallet':'$WAL','beweis':'$SIG'}"
pruefe "Signieradresse gross geschrieben" "{\"signing_address\":\"${ADR^^}\",\"signing_key_signature\":\"$SIG\"}" \
  "p['signing_address']=='$ADR'"

# Kettennachweis kaputt -> ohne Kette (das Netz prueft ihn ohnehin selbst)
pruefe "Kette: Zeit als Text" "{\"signing_address\":\"$ADR\",\"signing_key_signature\":\"$SIG\",\"bindung_zeit\":\"1792900000\",\"bindung_signatur_knoten\":\"$SIG\"}" "'bindung_zeit' not in p"
pruefe "Kette: Signatur kurz" "{\"signing_address\":\"$ADR\",\"signing_key_signature\":\"$SIG\",\"bindung_zeit\":1792900000,\"bindung_signatur_knoten\":\"0x12\"}" "'bindung_zeit' not in p"
pruefe "Kette: Zeit 0" "{\"signing_address\":\"$ADR\",\"signing_key_signature\":\"$SIG\",\"bindung_zeit\":0,\"bindung_signatur_knoten\":\"$SIG\"}" "'bindung_zeit' not in p"
pruefe "Kette: Fremdfeld wird nicht durchgereicht" "{\"signing_address\":\"$ADR\",\"signing_key_signature\":\"$SIG\",\"bindung_zeit\":1792900000,\"bindung_signatur_knoten\":\"$SIG\",\"wallet\":\"0x9999999999999999999999999999999999999999\"}" "p['wallet']=='$WAL' and len(p)==5"

# Keine Anfrage
pruefe "andere Signieradresse" "{\"signing_address\":\"0x3333333333333333333333333333333333333333\",\"signing_key_signature\":\"$SIG\"}" nein
pruefe "Beweis fehlt" "{\"signing_address\":\"$ADR\"}" nein
pruefe "Beweis kaputt" "{\"signing_address\":\"$ADR\",\"signing_key_signature\":\"0x\$(id)\"}" nein
pruefe "keine JSON-Antwort" "Service Unavailable" nein
pruefe "leer" "" nein

if [ "$fehler" = 0 ]; then echo "bindungsanfrage-skript: beide Skripte gleich, 7 Anfragen richtig gebaut, 5 Faelle ohne Anfrage"; fi
exit "$fehler"
