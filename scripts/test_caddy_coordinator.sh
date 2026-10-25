#!/usr/bin/env bash
# Vorpruefung in deploy/caddy-neuer-server.sh: Caddy wird nur umgeschaltet,
# wenn Vergleichsdienst und Proof-Server genau den Schluessel anerkennen, mit
# dem der Coordinator auf C1 unterschreibt. Der Test fuehrt den Block aus dem
# Skript selbst aus (zwischen dem Marker und "caddy validate"), mit einem
# nachgebauten docker -- keine Kopie der Logik.
set -euo pipefail
cd "$(dirname "$0")/.."
SKRIPT=deploy/caddy-neuer-server.sh
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

grep -q '^# --- coordinator_anerkannt anfang' "$SKRIPT" || { echo "Marker fehlt in $SKRIPT"; exit 1; }
sed -n '/^# --- coordinator_anerkannt anfang/,/caddy validate/p' "$SKRIPT" | sed '$d' > "$T/block.sh"
grep -q 'coordinator_anerkannt "\$KOORD"' "$T/block.sh" || { echo "Pruefung fehlt im Block"; exit 1; }

mkdir -p "$T/bin"
cat > "$T/bin/docker" <<'EOF'
#!/usr/bin/env bash
echo "$*" >> "$FAKE/aufrufe"
case "$1" in
  exec) [ -f "$FAKE/inventory" ] || exit 1; cat "$FAKE/inventory" ;;
  inspect) [ -f "$FAKE/env-$2" ] || exit 1; cat "$FAKE/env-$2" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$T/bin/docker"

K=$(printf 'a%.0s' $(seq 1 63))b                  # Schluessel des Coordinators
A=$(printf 'c%.0s' $(seq 1 64))                   # ein anderer
GROSS=$(printf '%s' "$K" | tr 'a-f' 'A-F')

fehler=0
fall() { # $1 Name, $2 erwarteter Exit, $3 (optional) erwartete Meldung; Aufbau als Variablen:
         # INV (JSON oder "-" fuer unerreichbar), M und P (Liste oder "-" fuer fehlenden Container),
         # GLEICH=1 Caddyfile unveraendert
  local name=$1 soll=$2 ist
  local d="$T/f"; rm -rf "$d"; mkdir -p "$d"
  [ "$INV" = - ] || printf '%s' "$INV" > "$d/inventory"
  [ "$M" = - ] || printf 'PATH=/x\nCOORDINATOR_PUBLIC_KEYS=%s\nWALLET_TAG_KEY=geheim\n' "$M" > "$d/env-aequitas-matching"
  [ "$P" = - ] || printf 'COORDINATOR_PUBLIC_KEYS=%s\n' "$P" > "$d/env-proof-server"
  printf 'neu\n' > "$d/neu"
  if [ "${GLEICH:-0}" = 1 ]; then cp "$d/neu" "$d/cf"; else printf 'alt\n' > "$d/cf"; fi
  set +e
  FAKE="$d" PATH="$T/bin:$PATH" CF="$d/cf" NEU="$d/neu" \
    bash -c 'set -euo pipefail; source "$1"' _ "$T/block.sh" > "$d/out" 2>&1
  ist=$?
  set -e
  if [ "$ist" = "$soll" ] && ! grep -q geheim "$d/out" && { [ -z "${3:-}" ] || grep -qF -- "$3" "$d/out"; }; then
    echo "ok   $name"
  else
    echo "FAIL $name (Exit $ist, erwartet $soll)"; sed 's/^/     /' "$d/out"; fehler=1
  fi
  if [ "${GLEICH:-0}" = 1 ] && [ -s "$d/aufrufe" ]; then
    echo "FAIL $name: docker aufgerufen, obwohl Caddyfile unveraendert"; fehler=1
  fi
}
inv() { printf '{"bereit_fuer_echtbetrieb":{"attestation_public_key":"%s"}}' "$1"; }

# Gutfaelle
INV=$(inv "$K") M="$K" P="$K" GLEICH=0 fall "beide erkennen an" 0
INV=$(inv "$K") M="$A, $GROSS" P=" $K ,$A" GLEICH=0 fall "in Listen mit Leerzeichen und Grossbuchstaben" 0
INV=$(inv "$GROSS") M="$K" P="$K" GLEICH=0 fall "Coordinator meldet Grossbuchstaben" 0
INV=- M=- P=- GLEICH=1 fall "Caddyfile unveraendert: keine Pruefung" 0

# Angriffe und Fehler: alle abgelehnt
INV=$(inv "$K") M="$A" P="$K" GLEICH=0 fall "Vergleichsdienst kennt nur einen anderen" 1
INV=$(inv "$K") M="$K" P="$A" GLEICH=0 fall "Proof-Server kennt nur einen anderen" 1
INV=$(inv "$K") M="" P="$K" GLEICH=0 fall "Vergleichsdienst-Liste leer" 1
INV=$(inv "$K") M=- P="$K" GLEICH=0 fall "Vergleichsdienst-Container fehlt" 1
INV=$(inv "$K") M="$K" P=- GLEICH=0 fall "Proof-Server-Container fehlt" 1
INV=$(inv "$K") M="${K}00" P="$K" GLEICH=0 fall "Liste: Schluessel mit Anhang" 1
INV=$(inv "$K") M="${K:0:63}" P="$K" GLEICH=0 fall "Liste: Schluessel abgeschnitten" 1
INV=$(inv "$K") M="x$K" P="$K" GLEICH=0 fall "Liste: Schluessel mit Vorsatz" 1
INV=- M="$K" P="$K" GLEICH=0 fall "Coordinator nicht erreichbar" 1 "nennt keinen Schluessel"
INV='{}' M="$K" P="$K" GLEICH=0 fall "Coordinator nennt keinen Schluessel" 1 "nennt keinen Schluessel"
INV=$(inv "") M=",," P=",," GLEICH=0 fall "leerer Schluessel gegen leere Eintraege" 1 "nennt keinen Schluessel"
INV=$(inv "${K:0:63}") M="${K:0:63}" P="${K:0:63}" GLEICH=0 fall "63 Zeichen auf allen Seiten" 1 "nennt keinen Schluessel"
INV=$(inv "${K:0:63}g") M="${K:0:63}g" P="${K:0:63}g" GLEICH=0 fall "kein Hex auf allen Seiten" 1 "nennt keinen Schluessel"
INV=$(inv '*') M='*' P='*' GLEICH=0 fall "Stern auf allen Seiten" 1 "nennt keinen Schluessel"
INV='kein json' M="$K" P="$K" GLEICH=0 fall "Unsinn statt JSON" 1 "nennt keinen Schluessel"

# Die Funktion selbst: nur ein ganzer Eintrag aus 64 Zeichen Hex zaehlt.
( source <(sed -n '/^coordinator_anerkannt()/,/^}/p' "$T/block.sh")
  f=0
  coordinator_anerkannt "$K" "$A,$K" || { echo "FAIL Funktion: Gutfall"; f=1; }
  for paar in ":" ":,," "*:*" "${K:0:63}:${K:0:63}" "${K:0:63}g:${K:0:63}g" "$GROSS:$GROSS" "$K:${K}0" "$K:" "$K:$A"; do
    coordinator_anerkannt "${paar%%:*}" "${paar#*:}" && { echo "FAIL Funktion nimmt an: $paar"; f=1; }
  done
  [ "$f" = 0 ] && echo "ok   Funktion: 1 Gutfall, 9 abgelehnt"
  exit "$f" ) || fehler=1

[ "$fehler" = 0 ] && echo "alle Faelle wie erwartet"
exit "$fehler"
