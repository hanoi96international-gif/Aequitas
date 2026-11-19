#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer deploy/validator/quanten-messen.sh mit
# gefaelschtem id, setpriv und Python (kein root, kein IBM, kein qiskit).
# Geprueft wird, was das Skript selbst entscheidet: wer es starten darf,
# welche Basen es annimmt, dass es als Knotenbenutzer misst und dass der
# IBM-Schluessel nur in der Umgebung steht, nie auf der Befehlszeile.
set -euo pipefail
cd "$(dirname "$0")/.."
D=$(mktemp -d); trap 'rm -rf "$D"' EXIT
mkdir -p "$D/bin" "$D/opt/venv/bin" "$D/opt/quantengeld" "$D/opt/home" "$D/mq" "$D/etc"
# id: meldet $UID_FAKE (Standard 0 = root).
cat > "$D/bin/id" <<'EOF'
#!/usr/bin/env bash
[ "${1:-}" = "-u" ] && { echo "${UID_FAKE:-0}"; exit 0; }
exit 99
EOF
# setpriv: schreibt seine Optionen mit und fuehrt den Rest aus.
cat > "$D/bin/setpriv" <<'EOF'
#!/usr/bin/env bash
opts=()
while [ "$1" != "--" ]; do opts+=("$1"); shift; done
shift
printf '%s\n' "${opts[*]}" > "$PROT/setpriv"
exec "$@"
EOF
# Python: schreibt Argumente, HOME und den Schluessel aus der Umgebung mit.
cat > "$D/opt/venv/bin/python" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "$PROT/args"
printf '%s\n' "$HOME" > "$PROT/home"
printf '%s' "${IBM_QUANTUM_TOKEN:-}" > "$PROT/token"
EOF
chmod +x "$D/bin/id" "$D/bin/setpriv" "$D/opt/venv/bin/python"
: > "$D/opt/quantengeld/hardware_ibm.py"
for t in bash env cat rm printf flock sleep; do ln -s "$(command -v $t)" "$D/bin/$t"; done
LANG_OK="$(printf '01%.0s' $(seq 1 8192))"        # 16.384 Zeichen
LANG_ZU="$(printf '1%.0s' $(seq 1 16385))"         # eins zu viel
SCHLUESSEL="geheim-$RANDOM-ibm"
printf '%s' "$SCHLUESSEL" > "$D/etc/token"
export PATH="$D/bin" PROT="$D" QS_OPT="$D/opt" QS_VERZEICHNIS="$D/mq" QS_SCHLUESSEL="$D/etc/token" QS_SPERRE="$D/sperre"

lauf() { # erwartung beschreibung -- argumente [env...]
  local soll=$1 was=$2; shift 2
  local args=()
  while [ "$1" != "--" ]; do args+=("$1"); shift; done
  shift
  rm -f "$D/args" "$D/setpriv" "$D/home" "$D/token"
  if env "$@" bash deploy/validator/quanten-messen.sh "${args[@]}" >"$D/out" 2>"$D/err"; then e=ok; else e=halt; fi
  [ "$e" = "$soll" ] || { echo "FEHLER: $was -> $e, erwartet $soll"; cat "$D/err"; exit 1; }
  if [ "$soll" = halt ] && [ -e "$D/args" ]; then echo "FEHLER: $was -> trotzdem gemessen"; exit 1; fi
  echo "ok: $was -> $e"
}

lauf ok "Gutfall" 0110 --
[ "$(cat "$D/args")" = "-I $D/opt/quantengeld/hardware_ibm.py --ibm --s-abschnitt $D/mq --basen 0110" ] \
  || { echo "FEHLER: Aufruf: $(cat "$D/args")"; exit 1; }
[ "$(cat "$D/setpriv")" = "--reuid 10001 --regid 10001 --clear-groups" ] \
  || { echo "FEHLER: nicht als Knotenbenutzer: $(cat "$D/setpriv")"; exit 1; }
[ "$(cat "$D/home")" = "$D/opt/home" ] || { echo "FEHLER: HOME $(cat "$D/home")"; exit 1; }
[ "$(cat "$D/token")" = "$SCHLUESSEL" ] || { echo "FEHLER: Schluessel nicht in der Umgebung"; exit 1; }
for f in args out err; do
  case "$(cat "$D/$f")" in *"$SCHLUESSEL"*) echo "FEHLER: Schluessel in $f (Befehlszeile oder Log)"; exit 1 ;; esac
done

lauf ok "16.384 Stellen" "$LANG_OK" --
for b in "" 012 "01 1" "0101;id" "0101'" "\$(id)" "-0101" "$LANG_ZU"; do
  lauf halt "Basen '${b:0:12}'" "$b" --
done
lauf halt "zwei Argumente" 0101 0101 --
lauf halt "nicht root" 0101 -- UID_FAKE=1000
lauf halt "kein Schluessel" 0101 -- QS_SCHLUESSEL="$D/etc/fehlt"
: > "$D/etc/leer"; lauf halt "leerer Schluessel" 0101 -- QS_SCHLUESSEL="$D/etc/leer"
lauf halt "nicht eingerichtet" 0101 -- QS_OPT="$D/fehlt"

# --warteschlange: misst die Vormerkungen des Knotens, als Knotenbenutzer,
# Zaehler im HOME des Knotenbenutzers, Tagesgrenze aus QS_JE_TAG.
lauf ok "Warteschlange" --warteschlange --
[ "$(cat "$D/args")" = "-I $D/opt/quantengeld/hardware_ibm.py --ibm --s-warteschlange $D/mq --zaehler $D/opt/home/messungen-je-tag.json --je-tag 8" ] \
  || { echo "FEHLER: Aufruf: $(cat "$D/args")"; exit 1; }
[ "$(cat "$D/setpriv")" = "--reuid 10001 --regid 10001 --clear-groups" ] \
  || { echo "FEHLER: Warteschlange nicht als Knotenbenutzer: $(cat "$D/setpriv")"; exit 1; }
[ "$(cat "$D/token")" = "$SCHLUESSEL" ] || { echo "FEHLER: Warteschlange: Schluessel nicht in der Umgebung"; exit 1; }
for f in args out err; do
  case "$(cat "$D/$f")" in *"$SCHLUESSEL"*) echo "FEHLER: Warteschlange: Schluessel in $f"; exit 1 ;; esac
done
lauf ok "Warteschlange, 3 am Tag" --warteschlange -- QS_JE_TAG=3
case "$(cat "$D/args")" in *" --je-tag 3") ;; *) echo "FEHLER: Tagesgrenze nicht weitergegeben: $(cat "$D/args")"; exit 1 ;; esac
for j in 0 65 100 -1 abc "8;id" "8 9"; do
  lauf halt "Warteschlange, QS_JE_TAG '$j'" --warteschlange -- QS_JE_TAG="$j"
done
lauf halt "Warteschlange mit Argument" --warteschlange 0101 --
lauf halt "Warteschlange nicht root" --warteschlange -- UID_FAKE=1000
lauf halt "Warteschlange ohne Schluessel" --warteschlange -- QS_SCHLUESSEL="$D/etc/fehlt"
# Laeuft schon einer (Sperre gehalten): der zweite misst nicht.
flock "$D/sperre" sleep 5 & halter=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do flock -n "$D/sperre" true 2>/dev/null || break; sleep 0.1; done
lauf ok "Warteschlange laeuft schon" --warteschlange --
[ ! -e "$D/args" ] || { echo "FEHLER: zweiter Lauf hat trotz Sperre gemessen"; exit 1; }
kill "$halter" 2>/dev/null || true; wait "$halter" 2>/dev/null || true
echo "alle Faelle ok"
