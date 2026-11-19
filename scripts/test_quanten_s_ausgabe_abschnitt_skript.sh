#!/usr/bin/env bash
# Missbrauchs- und Gutfaelle fuer deploy/quanten-s-ausgabe-abschnitt.sh mit
# gefaelschtem docker und chown im PATH (kein Container, kein Knoten, kein root).
set -euo pipefail
cd "$(dirname "$0")/.."
SKRIPT="$PWD/deploy/quanten-s-ausgabe-abschnitt.sh"
D=$(mktemp -d); trap 'chmod -R u+w "$D" 2>/dev/null || true; rm -rf "$D"' EXIT
mkdir -p "$D/bin"
# Gefaelschtes docker:
# - inspect .State.Running meldet $LAEUFT, inspect .Config.Env die Umgebung $UMGEBUNG;
# - exec schreibt Argumente und stdin mit, prueft, dass die Messung unter dem
#   richtigen Namen mit 0600 liegt, und "verbraucht" sie wie der Dienst
#   (VERBRAUCHEN=1), bevor es ein Ergebnis ausgibt.
cat > "$D/bin/docker" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  inspect)
    case "$3" in
      *State.Running*) [ "${LAEUFT:-true}" = "true" ] && echo true || echo false ;;
      *Config.Env*) printf '%s\n' "PATH=/usr/bin" "${UMGEBUNG-AEQUITAS_QS_MESSQUELLE=/data/quanten}" ;;
    esac ;;
  exec)
    printf '%s\n' "$*" > "$PROT/args"; cat > "$PROT/stdin"
    f="$PROT/verz/$PROT_NAME.json"
    if [ -f "$f" ]; then stat -c '%a' "$f" > "$PROT/modus"; cp "$f" "$PROT/gesehen"; fi
    [ "${VERBRAUCHEN:-1}" = 1 ] && rm -f "$f"
    echo '{"vorlage":"eingeloest"}'; exit "${EXIT_EXEC:-0}" ;;
  *) exit 99 ;;
esac
EOF
# Gefaelschtes chown: schreibt mit, aendert nichts (der Test laeuft nicht als root).
cat > "$D/bin/chown" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$PROT/chown"
EOF
chmod +x "$D/bin/docker" "$D/bin/chown"
ECHT_CHOWN=$(command -v chown)
for t in bash env cat rm stat printf head tr ln grep dirname mktemp chmod cp id mkdir rmdir ls; do ln -s "$(command -v $t)" "$D/bin/$t"; done
export PATH="$D/bin" PROT="$D"
# Der Knotenbenutzer im Test: die eigene uid -- als root (Arbeitsumgebung) eine
# feste andere, denn das Skript weist uid 0 zu Recht ab.
ICH=$(id -u)
[ "$ICH" != 0 ] || ICH=4243
NAME=$(printf '%064d' 0 | tr 0 a)
export PROT_NAME="$NAME"
MESS='{"u":"01","t":"10","y":"01","x":"10","wo":"Test","job":""}'
BASEN='{"n":2,"basen":["01"]}'
neu() { # frisches Messverzeichnis und frische Eingaben
  rm -rf "$D/verz"; mkdir "$D/verz"; chmod 700 "$D/verz"
  [ "$(id -u)" != 0 ] || "$ECHT_CHOWN" "$ICH" "$D/verz"
  printf '%s' "$MESS" > "$D/m.json"; printf '%s' "$BASEN" > "$D/b.json"
  rm -f "$D/args" "$D/stdin" "$D/chown" "$D/modus" "$D/gesehen"
}
lauf() { # erwartung beschreibung [env...]
  local soll=$1 was=$2; shift 2
  if env QS_DATEI="$D/m.json" QS_BASEN="$D/b.json" QS_VERZ="$D/verz" QS_UID="$ICH" QS_NAME="$NAME" "$@" \
      bash "$SKRIPT" >"$D/out" 2>"$D/err"; then e=ok; else e=halt; fi
  [ "$e" = "$soll" ] || { echo "FEHLER: $was -> $e, erwartet $soll"; cat "$D/err"; exit 1; }
  [ ! -e "$D/m.json" ] && [ ! -e "$D/b.json" ] || { echo "FEHLER: $was -> Eingaben nicht geloescht"; exit 1; }
  echo "ok: $was -> $e"
}
kein_exec() { [ ! -e "$D/args" ] || { echo "FEHLER: $1 -> docker exec trotzdem"; exit 1; }; }
nichts_abgelegt() { [ -z "$(ls -A "$D/verz" 2>/dev/null)" ] || { echo "FEHLER: $1 -> Reste im Messverzeichnis: $(ls -A "$D/verz")"; exit 1; }; }

# Gutfall
neu; lauf ok "Gutfall"
grep -q -- '-u 10001:10001 aequitas-node ./aequitasd quanten-s-ausgabe --api http://127.0.0.1:8080 --deckung 1000000 --frist 15m --toleranz 2 --stichprobe-v2' "$D/args" \
  || { echo "FEHLER: Aufruf: $(cat "$D/args")"; exit 1; }
[ "$(cat "$D/stdin")" = "$BASEN" ] || { echo "FEHLER: Basen nicht ueber stdin"; exit 1; }
[ "$(cat "$D/gesehen")" = "$MESS" ] || { echo "FEHLER: Messung nicht unter dem Namen der Basen abgelegt"; exit 1; }
[ "$(cat "$D/modus")" = 600 ] || { echo "FEHLER: Messung nicht 0600"; exit 1; }
grep -qx "$ICH:$ICH -- $D/verz/.neu."* "$D/chown" || { echo "FEHLER: chown auf den Knotenbenutzer fehlt: $(cat "$D/chown" 2>/dev/null)"; exit 1; }
grep -q eingeloest "$D/out" || { echo "FEHLER: Ergebnis fehlt"; exit 1; }
nichts_abgelegt "Gutfall"
neu; lauf ok "Deckung 2 AEQ, Toleranz 4" DECKUNG=2000000 TOLERANZ=4
grep -q -- '--deckung 2000000 --frist 15m --toleranz 4 --stichprobe-v2' "$D/args" || { echo "FEHLER: Deckung/Toleranz nicht weitergegeben"; exit 1; }

# Eingaben: Abbruch ohne exec und ohne abgelegte Messung
for dk in 999999 10000001 '1000000;id' ' 1000000' 01000000 x ''; do
  neu; lauf halt "Deckung '$dk'" DECKUNG="$dk"; kein_exec "Deckung '$dk'"; nichts_abgelegt "Deckung '$dk'"
done
for tl in 1 5 10 '2;id' ' 2' x ''; do
  neu; lauf halt "Toleranz '$tl'" TOLERANZ="$tl"; kein_exec "Toleranz '$tl'"; nichts_abgelegt "Toleranz '$tl'"
done
for nm in '' "${NAME:1}" "${NAME}0" "$(printf '%064d' 0 | tr 0 A)" "../$(printf '%061d' 0)" "${NAME:0:63}/"; do
  neu; lauf halt "Name '$nm'" QS_NAME="$nm"; kein_exec "Name '$nm'"; nichts_abgelegt "Name '$nm'"
done
neu; rm -f "$D/m.json"; lauf halt "keine Messung"; kein_exec "keine Messung"
neu; rm -f "$D/b.json"; lauf halt "keine Basen"; kein_exec "keine Basen"
neu; rm -f "$D/m.json"; printf '%s' "$MESS" > "$D/echt.json"; ln -s "$D/echt.json" "$D/m.json"; lauf halt "Messung als Verweis"
[ -e "$D/echt.json" ] || { echo "FEHLER: Ziel des Verweises geloescht"; exit 1; }; nichts_abgelegt "Messung als Verweis"
neu; head -c 1048577 /dev/zero | tr '\0' ' ' > "$D/m.json"; lauf halt "Messung zu gross"; kein_exec "Messung zu gross"
neu; head -c 1048577 /dev/zero | tr '\0' ' ' > "$D/b.json"; lauf halt "Basen zu gross"; kein_exec "Basen zu gross"

# Messverzeichnis und Knoten
neu; chmod 755 "$D/verz"; lauf halt "Messverzeichnis 0755"; kein_exec "Messverzeichnis 0755"; nichts_abgelegt "Messverzeichnis 0755"
neu; lauf halt "Messverzeichnis gehoert jemand anderem" QS_UID=4242; kein_exec "fremder Besitzer"
for u in 0 '' x '10001;id' -1; do neu; lauf halt "QS_UID '$u'" QS_UID="$u"; kein_exec "QS_UID '$u'"; nichts_abgelegt "QS_UID '$u'"; done
neu; rm -rf "$D/verz"; mkdir "$D/echtverz"; chmod 700 "$D/echtverz"; [ "$(id -u)" != 0 ] || "$ECHT_CHOWN" "$ICH" "$D/echtverz"; ln -s "$D/echtverz" "$D/verz"; lauf halt "Messverzeichnis als Verweis"
kein_exec "Messverzeichnis als Verweis"; [ -z "$(ls -A "$D/echtverz")" ] || { echo "FEHLER: in das Ziel des Verweises abgelegt"; exit 1; }; rm -f "$D/verz"; rmdir "$D/echtverz"
neu; rm -rf "$D/verz"; lauf halt "kein Messverzeichnis"; kein_exec "kein Messverzeichnis"
neu; lauf halt "Container laeuft nicht" LAEUFT=false; kein_exec "Container laeuft nicht"; nichts_abgelegt "Container laeuft nicht"
neu; lauf halt "Knoten ohne Messquelle" UMGEBUNG=; kein_exec "ohne Messquelle"; nichts_abgelegt "ohne Messquelle"
neu; lauf halt "Knoten mit anderer Messquelle" UMGEBUNG=AEQUITAS_QS_MESSQUELLE=/data/anders; kein_exec "andere Messquelle"
# Es liegt schon eine Messung zu diesen Basen: nicht ueberschreiben, nicht loeschen
neu; printf 'alt' > "$D/verz/$NAME.json"; lauf halt "Messung zu den Basen liegt schon"; kein_exec "schon da"
[ "$(cat "$D/verz/$NAME.json")" = alt ] || { echo "FEHLER: vorhandene Messung veraendert oder geloescht"; exit 1; }
[ "$(ls -A "$D/verz")" = "$NAME.json" ] || { echo "FEHLER: Reste neben der vorhandenen Messung: $(ls -A "$D/verz")"; exit 1; }
neu; ln -s "$D/nirgends" "$D/verz/$NAME.json"; lauf halt "Verweis unter dem Zielnamen"; kein_exec "Verweis unter dem Zielnamen"

# Werkzeug scheitert: die nicht verbrauchte Messung wird entfernt
neu; lauf halt "Werkzeug scheitert vor dem Dienst" EXIT_EXEC=1 VERBRAUCHEN=0
nichts_abgelegt "Werkzeug scheitert vor dem Dienst"
neu; lauf halt "Werkzeug scheitert nach dem Dienst" EXIT_EXEC=1 VERBRAUCHEN=1
nichts_abgelegt "Werkzeug scheitert nach dem Dienst"
echo "alle Faelle ok"
