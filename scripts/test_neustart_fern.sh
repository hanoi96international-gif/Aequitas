#!/usr/bin/env bash
# Missbrauchstest fuer .github/neustart/fern.sh: ein Skript, dessen Rest von
# einem Befehl verschluckt wird (wie docker exec -i am 21.10.2026), muss rot
# werden, auch wenn bash dort mit 0 endet. Ein ssh-Ersatz fuehrt den
# Fernbefehl lokal aus, mit derselben Standardeingabe.
set -euo pipefail
WURZEL="$(cd "$(dirname "$0")/.." && pwd)"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
cat > "$T/ssh" <<'SH'
#!/usr/bin/env bash
eval "${@: -1}"
SH
chmod +x "$T/ssh"
export SSH="$T/ssh"
FEHLER=0
fall() { # fall NAME SOLL(gruen|rot) SKRIPT
  printf '%s\n' "$3" > "$T/s.sh"
  if bash "$WURZEL/.github/neustart/fern.sh" host.invalid "BOX=C1" "$T/s.sh" > "$T/aus" 2>&1; then ist=gruen; else ist=rot; fi
  if [ "$ist" = "$2" ]; then echo "ok   $1 ($ist)"; else echo "FEHL $1: $ist, erwartet $2"; cat "$T/aus"; FEHLER=1; fi
}
fall "laeuft durch"               gruen 'set -euo pipefail; echo "BOX=$BOX"; echo fertig'
fall "Eingabe verschluckt"        rot   'set -euo pipefail; echo vorher; cat >/dev/null; echo "wird nie gelesen"'
fall "verschluckt wie docker -i"  rot   'set -euo pipefail; zaehlen() { cat >/dev/null || true; echo "proof_bio_hashes 0"; }; zaehlen; echo loeschen'
fall "bricht mit Fehler ab"       rot   'set -euo pipefail; echo los; false; echo nie'
fall "exit 0 vor dem Ende"        rot   'echo halb; exit 0; echo nie'
fall "eigene Eingabe ist erlaubt" gruen 'set -euo pipefail; n="$(printf "a\nb\n" | wc -l)"; [ "$n" = 2 ]; cat </dev/null; echo fertig'
exit "$FEHLER"
