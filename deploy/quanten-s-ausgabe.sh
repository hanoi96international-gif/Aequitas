#!/usr/bin/env bash
# Die erste echte Ausgabe eines S-Tokens (Quantengeld Variante S) auf C1.
# Laeuft ueber .github/workflows/quanten-s-ausgabe.yml (Freigabe noetig).
#
# Die Messung (hardware_ibm.py --s-messen, IBM als Geraet der Inhaberin)
# liegt in $QS_DATEI. Das Werkzeug laeuft IM laufenden Knoten-Container,
# als Knotenbenutzer (nicht root), mit dem Schluessel des Knotens aus dessen
# Umgebung -- der Schluessel verlaesst den Container nicht und steht in
# keinem Protokoll. Die Messung geht nur ueber stdin hinein und wird danach
# geloescht, auch im Fehlerfall.
#
#   DECKUNG  Mikro-AEQ, 1000000 bis 10000000 (Standard 1000000 = 1 AEQ)
#
# Ausgabe: das Ergebnis als JSON auf stdout (K, Stellen, Urteile; nichts
# Geheimes), der Verlauf auf stderr. Exit != 0, wenn ein Schritt scheitert
# oder die Doppelvorlage NICHT abgewiesen wurde.
set -euo pipefail
QS_DATEI="${QS_DATEI:-/root/qs-messung.json}"
CONTAINER="${CONTAINER:-aequitas-node}"
DECKUNG="${DECKUNG:-1000000}"
trap 'rm -f -- "$QS_DATEI"' EXIT

[[ "$DECKUNG" =~ ^[1-9][0-9]{6,7}$ ]] && [ "$DECKUNG" -ge 1000000 ] && [ "$DECKUNG" -le 10000000 ] \
  || { echo "ABBRUCH: DECKUNG $DECKUNG (1000000 bis 10000000 Mikro-AEQ)" >&2; exit 2; }
[ -f "$QS_DATEI" ] && [ ! -L "$QS_DATEI" ] || { echo "ABBRUCH: keine Messung in $QS_DATEI" >&2; exit 2; }
[ "$(stat -c %s "$QS_DATEI")" -le 1048576 ] || { echo "ABBRUCH: Messung zu gross" >&2; exit 2; }
# Den Inhalt prueft das Werkzeug streng (JSON ohne fremde Felder, Bits,
# Laengen); jq ist auf C1 nicht installiert.
[ "$(docker inspect -f '{{.State.Running}}' "$CONTAINER" 2>/dev/null)" = "true" ] \
  || { echo "ABBRUCH: Container $CONTAINER laeuft nicht" >&2; exit 2; }

docker exec -i -u 10001:10001 "$CONTAINER" ./aequitasd quanten-s-ausgabe \
  --api http://127.0.0.1:8080 --deckung "$DECKUNG" --frist 15m < "$QS_DATEI"
