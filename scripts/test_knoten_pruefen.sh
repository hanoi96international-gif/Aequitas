#!/usr/bin/env bash
# Gut- und Missbrauchsfaelle fuer deploy/knoten/pruefen.sh, ohne Netz: eine
# Attrappe fuer curl antwortet je URL aus Dateien.
#   - bereit (0): Netz und Knoten gleich hoch, gebunden, Verifier und Wache ok
#   - noch nicht fertig (2): holt auf / nicht gebunden
#   - Fehler (1): Knoten stumm, Verifier stumm, Wache != 200, Knoten weit VOR
#     dem Netz, kaputte Hoehe/Adresse (zaehlt nie als ok), falsche IP als Argument
set -euo pipefail
cd "$(dirname "$0")/.."
QUELLE="$PWD/deploy/knoten/pruefen.sh"
D="$(mktemp -d)"; trap 'rm -rf "$D"' EXIT
mkdir -p "$D/deploy/knoten" "$D/deploy/verifier" "$D/bin" "$D/antwort"
cp "$QUELLE" "$D/deploy/knoten/pruefen.sh"
echo "VERIFIER_HOST=203-0-113-10.sslip.io" > "$D/deploy/verifier/.env"

# curl-Attrappe: letzte Angabe ist die URL; Antwort aus $ANTWORT/<url mit _>.
# Mit -w '%{http_code}' gibt sie den Code aus $ANTWORT/<url>.code aus.
cat > "$D/bin/curl" <<'EOF'
#!/usr/bin/env bash
url="${@: -1}"; f="$ANTWORT/$(printf '%s' "$url" | tr -c 'A-Za-z0-9' '_')"
case " $* " in
  *" -w "*) cat "$f.code" 2>/dev/null || echo 000; exit 0 ;;
esac
[ -f "$f" ] || exit 22
cat "$f"
EOF
chmod +x "$D/bin/curl"
setze() { printf '%s' "$2" > "$D/antwort/$(printf '%s' "$1" | tr -c 'A-Za-z0-9' '_')"; }
code() { printf '%s' "$2" > "$D/antwort/$(printf '%s' "$1" | tr -c 'A-Za-z0-9' '_').code"; }
weg() { rm -f "$D/antwort/$(printf '%s' "$1" | tr -c 'A-Za-z0-9' '_')"; }

ADDR=0x00000000000000000000000000000000000000aa
WALLET=0x00000000000000000000000000000000000000bb
NETZ=https://aequitas.digital
grund() {
  rm -f "$D/antwort/"*
  setze "$NETZ/api/status" '{"height": 1000, "chain_id": "aequitas-1"}'
  setze "http://127.0.0.1:8080/api/status" "{\"height\": 995, \"validator_address\": \"$ADDR\"}"
  setze "http://203.0.113.10:8080/api/status" "{\"height\": 995, \"validator_address\": \"$ADDR\"}"
  setze "$NETZ/api/validator-binding?signing_address=$ADDR" "{\"signing_address\": \"$ADDR\", \"human_wallet\": \"$WALLET\"}"
  setze "https://203-0-113-10.sslip.io/health" '{"ok": true}'
  code "http://127.0.0.1:8080/api/wache" 200
}

lauf() { # erwarteter_code beschreibung [arg]
  local erw="$1" was="$2"; shift 2
  local rc=0
  ( cd "$D/deploy/knoten" && env PATH="$D/bin:$PATH" ANTWORT="$D/antwort" bash pruefen.sh "$@" ) > "$D/ausgabe" 2>&1 || rc=$?
  if [ "$rc" != "$erw" ]; then
    echo "FEHLER ($was): erwartet $erw, bekommen $rc"; sed -e 's/\x1b\[[0-9;]*m//g' "$D/ausgabe"; exit 1
  fi
  echo "ok: $was -> $rc"
}

grund; lauf 0 "bereit (auf dem Server)"
grund; lauf 0 "bereit (von aussen)" 203.0.113.10
grep -q "Selbstpruefung" "$D/ausgabe" && { echo "FEHLER: von aussen darf /api/wache nicht geprueft werden"; exit 1; }

grund; setze "http://127.0.0.1:8080/api/status" "{\"height\": 900, \"validator_address\": \"$ADDR\"}"; lauf 2 "holt auf"
grund; weg "$NETZ/api/validator-binding?signing_address=$ADDR"; lauf 2 "nicht gebunden"
grund; setze "$NETZ/api/validator-binding?signing_address=$ADDR" '{"human_wallet": "0xnicht"}'; lauf 2 "Bindung unlesbar = nicht gebunden"

grund; weg "http://127.0.0.1:8080/api/status"; lauf 1 "Knoten stumm"
grund; weg "https://203-0-113-10.sslip.io/health"; lauf 1 "Verifier stumm"
grund; code "http://127.0.0.1:8080/api/wache" 503; lauf 1 "Wache meldet Problem"
grund; weg "$NETZ/api/status"; lauf 1 "Netz stumm"
grund; setze "http://127.0.0.1:8080/api/status" "{\"height\": 5000, \"validator_address\": \"$ADDR\"}"; lauf 1 "Knoten weit vor dem Netz (anderer Zweig)"
grund; setze "http://127.0.0.1:8080/api/status" "{\"height\": \"995\", \"validator_address\": \"$ADDR\"}"; lauf 1 "Hoehe als Text zaehlt nicht"
grund; setze "http://127.0.0.1:8080/api/status" "{\"height\": 1234567890123, \"validator_address\": \"$ADDR\"}"; lauf 1 "Hoehe mit 13 Stellen zaehlt nicht"
grund; setze "http://127.0.0.1:8080/api/status" '{"height": 995, "validator_address": "0x12"}'; lauf 1 "kaputte Signieradresse"
grund; echo "VERIFIER_HOST=evil.example.com" > "$D/deploy/verifier/.env"; setze "https://evil.example.com/health" '{"ok": true}'
lauf 1 "fremde Verifier-Adresse in .env wird nicht abgefragt"
grep -q "Verifier-Adresse unbekannt" "$D/ausgabe" || { echo "FEHLER: fremde Verifier-Adresse abgefragt"; cat "$D/ausgabe"; exit 1; }
echo "VERIFIER_HOST=203-0-113-10.sslip.io" > "$D/deploy/verifier/.env"
# Ungueltige Ziele werden abgewiesen, BEVOR irgendetwas abgefragt wird.
for z in "evil.example.com" "300.1.1.1" '1.2.3.4;id' "1.2.3" "01.2.3.4.5"; do
  grund; setze "http://$z:8080/api/status" "{\"height\": 1000, \"validator_address\": \"$ADDR\"}"
  lauf 1 "kein gueltiges Ziel: $z" "$z"
  grep -q "Keine IPv4" "$D/ausgabe" || { echo "FEHLER: $z nicht als ungueltig abgewiesen"; cat "$D/ausgabe"; exit 1; }
done
echo "alle Faelle ok"
