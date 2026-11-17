#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer deploy/quanten-s-ausgabe.sh mit einem
# gefaelschten docker im PATH (kein Container, kein Knoten).
set -euo pipefail
cd "$(dirname "$0")/.."
D=$(mktemp -d); trap 'rm -rf "$D"' EXIT
mkdir -p "$D/bin"
# Gefaelschtes docker: inspect meldet $LAEUFT; exec schreibt seine Argumente
# und stdin mit und gibt ein Ergebnis aus.
cat > "$D/bin/docker" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  inspect) [ "${LAEUFT:-true}" = "true" ] && echo true || echo false ;;
  exec) printf '%s\n' "$*" > "$PROT/args"; cat > "$PROT/stdin"; echo '{"vorlage":"eingeloest"}'; exit "${EXIT_EXEC:-0}" ;;
  *) exit 99 ;;
esac
EOF
chmod +x "$D/bin/docker"
# Ohne jq im PATH (wie auf C1): das Skript darf es nicht brauchen.
for t in bash env cat rm stat printf head tr ln grep dirname mktemp; do ln -s "$(command -v $t)" "$D/bin/$t"; done
export PATH="$D/bin" PROT="$D"
GUT='{"n":2,"u":"01","t":"01","y":"01","x":"01"}'
lauf() { # erwartung beschreibung [env...]
  local soll=$1 was=$2; shift 2
  rm -f "$D/args" "$D/stdin"
  if env "$@" QS_DATEI="$D/m.json" bash deploy/quanten-s-ausgabe.sh >"$D/out" 2>"$D/err"; then e=ok; else e=halt; fi
  [ "$e" = "$soll" ] || { echo "FEHLER: $was -> $e, erwartet $soll"; cat "$D/err"; exit 1; }
  [ ! -e "$D/m.json" ] || { echo "FEHLER: $was -> Messung nicht geloescht"; exit 1; }
  echo "ok: $was -> $e"
}
printf '%s' "$GUT" > "$D/m.json"; lauf ok "Gutfall"
grep -q -- '-u 10001:10001 aequitas-node ./aequitasd quanten-s-ausgabe --api http://127.0.0.1:8080 --deckung 1000000 --frist 15m' "$D/args" \
  || { echo "FEHLER: Aufruf: $(cat "$D/args")"; exit 1; }
[ "$(cat "$D/stdin")" = "$GUT" ] || { echo "FEHLER: Messung nicht ueber stdin"; exit 1; }
grep -q eingeloest "$D/out" || { echo "FEHLER: Ergebnis fehlt"; exit 1; }
printf '%s' "$GUT" > "$D/m.json"; lauf ok "Deckung 2 AEQ" DECKUNG=2000000
grep -q -- '--deckung 2000000 ' "$D/args" || { echo "FEHLER: Deckung nicht weitergegeben"; exit 1; }
for dk in 999999 10000001 '1000000;id' ' 1000000' 01000000 x; do
  printf '%s' "$GUT" > "$D/m.json"; lauf halt "Deckung '$dk'" DECKUNG="$dk"
  [ ! -e "$D/args" ] || { echo "FEHLER: docker exec trotz Deckung '$dk'"; exit 1; }
done
printf '' > "$D/m.json"; lauf halt "leere Messung" EXIT_EXEC=1   # das Werkzeug lehnt ab
head -c 1048577 /dev/zero | tr '\0' ' ' > "$D/m.json"; lauf halt "zu gross"
lauf halt "keine Messung"
printf '%s' "$GUT" > "$D/echt.json"; ln -s "$D/echt.json" "$D/m.json"; lauf halt "Symlink"
[ -e "$D/echt.json" ] || { echo "FEHLER: Ziel des Symlinks geloescht"; exit 1; }
printf '%s' "$GUT" > "$D/m.json"; lauf halt "Container laeuft nicht" LAEUFT=false
printf '%s' "$GUT" > "$D/m.json"; lauf halt "Werkzeug scheitert (Messung trotzdem weg)" EXIT_EXEC=1
echo "alle Faelle ok"
