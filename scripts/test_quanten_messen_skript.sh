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
for t in bash env cat rm printf; do ln -s "$(command -v $t)" "$D/bin/$t"; done
LANG_OK="$(printf '01%.0s' $(seq 1 8192))"        # 16.384 Zeichen
LANG_ZU="$(printf '1%.0s' $(seq 1 16385))"         # eins zu viel
SCHLUESSEL="geheim-$RANDOM-ibm"
printf '%s' "$SCHLUESSEL" > "$D/etc/token"
export PATH="$D/bin" PROT="$D" QS_OPT="$D/opt" QS_VERZEICHNIS="$D/mq" QS_SCHLUESSEL="$D/etc/token"

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
echo "alle Faelle ok"
