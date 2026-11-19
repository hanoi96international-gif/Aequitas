#!/usr/bin/env bash
# Ausgabe eines S-Tokens (Quantengeld Variante S) auf C1 im Abschnitts-Weg mit
# Stichprobe v2 (Stichprobe erst nach der Bindung, DEZENTRAL.md 16).
# Laeuft ueber .github/workflows/quanten-s-ausgabe.yml, Weg abschnitte-v2
# (Freigaben quantenhardware und produktion).
#
# C1 ist Ausgeber und einziger Validator. Sein Dienst praepariert den einen
# Abschnitt aus der Messung, die hier in sein Messverzeichnis gelegt wird
# (AEQUITAS_QS_MESSQUELLE=/data/quanten im Container, auf dem Server
# QS_VERZ). Der Dienst liest genau die Datei zu den Basen der Inhaberin,
# prueft sie, ENTFERNT sie und verwendet sie erst danach -- einmal.
#
#   QS_DATEI   die Messung (ein Datensatz von hardware_ibm.py --s-abschnitt)
#   QS_BASEN   {"n": N, "basen": ["..."]} fuer das Werkzeug (stdin)
#   QS_NAME    sha256 der Basen als 64 Hex-Zeichen (Dateiname im Verzeichnis;
#              der Dienst prueft selbst, dass Name und Inhalt passen)
#   DECKUNG    Mikro-AEQ, 1000000 bis 10000000
#   TOLERANZ   2, 3 oder 4 (Prozent)
#
# Ausgabe: das Ergebnis als JSON auf stdout (nichts Geheimes), der Verlauf
# auf stderr. Messung und Basen werden danach geloescht, auch im Fehlerfall;
# eine hier abgelegte, aber nicht verbrauchte Messung ebenso.
set -euo pipefail
QS_DATEI="${QS_DATEI:-/root/qs-abschnitt.json}"
QS_BASEN="${QS_BASEN:-/root/qs-basen.json}"
QS_VERZ="${QS_VERZ:-/srv/aequitas-quanten}"
QS_UID="${QS_UID-10001}"
CONTAINER="${CONTAINER:-aequitas-node}"
DECKUNG="${DECKUNG-1000000}"
TOLERANZ="${TOLERANZ-2}"
QS_NAME="${QS_NAME:-}"
ABGELEGT=""
TMP=""
aufraeumen() {
  rm -f -- "$QS_DATEI" "$QS_BASEN"
  [ -z "$TMP" ] || rm -f -- "$TMP"
  # Nur die eigene Datei, und nur, wenn der Dienst sie nicht verbraucht hat.
  [ -z "$ABGELEGT" ] || rm -f -- "$ABGELEGT"
}
trap aufraeumen EXIT
halt() { echo "ABBRUCH: $*" >&2; exit 2; }

[[ "$DECKUNG" =~ ^[1-9][0-9]{6,7}$ ]] && [ "$DECKUNG" -ge 1000000 ] && [ "$DECKUNG" -le 10000000 ] \
  || halt "DECKUNG $DECKUNG (1000000 bis 10000000 Mikro-AEQ)"
[[ "$TOLERANZ" =~ ^[234]$ ]] || halt "TOLERANZ $TOLERANZ (2, 3 oder 4)"
[[ "$QS_NAME" =~ ^[0-9a-f]{64}$ ]] || halt "QS_NAME: 64 Hex-Zeichen"
[[ "$QS_UID" =~ ^[1-9][0-9]{0,9}$ ]] || halt "QS_UID"
for f in "$QS_DATEI" "$QS_BASEN"; do
  [ -f "$f" ] && [ ! -L "$f" ] || halt "keine Datei $f"
  [ "$(stat -c %s "$f")" -le 1048576 ] || halt "$f zu gross"
done
# Das Messverzeichnis: kein Verweis, gehoert dem Knotenbenutzer, nur er darf
# hinein (richtet deploy/deploy-c1.sh ein).
[ -d "$QS_VERZ" ] && [ ! -L "$QS_VERZ" ] || halt "kein Messverzeichnis $QS_VERZ"
[ "$(stat -c '%u %a' "$QS_VERZ")" = "$QS_UID 700" ] || halt "$QS_VERZ: Besitzer $QS_UID und 0700 verlangt"
[ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null)" = "true" ] \
  || halt "Container $CONTAINER laeuft nicht"
# Ohne Messquelle wiese der Dienst den Auftrag ab -- dann gar nicht erst
# die Messung ablegen.
docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$CONTAINER" 2>/dev/null \
  | grep -qx 'AEQUITAS_QS_MESSQUELLE=/data/quanten' || halt "der Knoten hat keine Messquelle (AEQUITAS_QS_MESSQUELLE)"

# Ablegen wie hardware_ibm.py s_messquelle_ablegen: erst unter eigenem Namen
# (0600, Knotenbenutzer), dann per ln auf den endgueltigen Namen -- ln
# scheitert, wenn es ihn schon gibt; nie wird eine Messung ueberschrieben.
ZIEL="$QS_VERZ/$QS_NAME.json"
[ ! -e "$ZIEL" ] && [ ! -L "$ZIEL" ] || halt "zu diesen Basen liegt schon eine Messung"
TMP=$(mktemp -p "$QS_VERZ" .neu.XXXXXXXX)
cat -- "$QS_DATEI" > "$TMP"
chmod 600 -- "$TMP"
chown "$QS_UID:$QS_UID" -- "$TMP"
ln -- "$TMP" "$ZIEL" || halt "zu diesen Basen liegt schon eine Messung"
ABGELEGT="$ZIEL"
rm -f -- "$TMP"
TMP=""

docker exec -i -u 10001:10001 "$CONTAINER" ./aequitasd quanten-s-ausgabe \
  --api http://127.0.0.1:8080 --deckung "$DECKUNG" --frist 15m --toleranz "$TOLERANZ" --stichprobe-v2 < "$QS_BASEN"
