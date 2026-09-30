# C1 ist nach dem Neustart bei null der einzige Blockproduzent (Betreiber,
# 30.09.2026). C1 signiert schon mit einem EIGENEN Schluessel, nicht mit der
# Wallet des Betreibers (Erkundung 30.09.: Signieradresse != NODE_OPERATOR_WALLET,
# Bindung gesetzt) -- er bleibt, und mit ihm die Bindung an den Betreiber.
# Hier wird nur AUTHORIZED_VALIDATORS auf C1 allein gesetzt; C2 wird spaeter
# nach dem Guide neu eingerichtet und dann wieder aufgenommen.
# Der private Schluessel wird nur im Proof-Server-Container in eine Adresse
# umgerechnet und nie ausgegeben.
set -euo pipefail
[ "${BESTAETIGT:-}" = "ALLE GUTHABEN WEG" ] || { echo "ABBRUCH: keine Bestaetigung"; exit 1; }
ENVF=/root/Aequitas/deploy/validator/.env
[ -f "$ENVF" ] || { echo "ABBRUCH: $ENVF fehlt"; exit 1; }
laeuft "$NODE" && { echo "ABBRUCH: $NODE laeuft noch -- erst leeren"; exit 1; }
laeuft "$PROOF" || { echo "ABBRUCH: $PROOF laeuft nicht (rechnet die Adresse mit ethers)"; exit 1; }

PK="$(grep -E '^RELAYER_PRIVATE_KEY=' "$ENVF" | tail -1 | cut -d= -f2-)"
[ -n "$PK" ] || { echo "ABBRUCH: RELAYER_PRIVATE_KEY fehlt in $ENVF"; exit 1; }
ADDR="$(printf '%s' "$PK" | docker exec -i "$PROOF" node -e "let s='';process.stdin.on('data',d=>s+=d).on('end',()=>{const {Wallet}=require('ethers');process.stdout.write(new Wallet(s.trim()).address.toLowerCase())})")"
unset PK
[[ "$ADDR" =~ ^0x[0-9a-f]{40}$ ]] || { echo "ABBRUCH: Signieradresse unlesbar"; exit 1; }

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p /root/backups && chmod 700 /root/backups
install -m 600 "$ENVF" "/root/backups/validator-env-vor-neustart-$STAMP"
tmp="$(mktemp)"
grep -vE '^AUTHORIZED_VALIDATORS=' "$ENVF" > "$tmp" || true
printf 'AUTHORIZED_VALIDATORS=%s\n' "$ADDR" >> "$tmp"
install -m 600 "$tmp" "$ENVF"; rm -f "$tmp"

echo "Signieradresse von C1 (bleibt): $ADDR"
echo "AUTHORIZED_VALIDATORS = $ADDR (C1 allein)"
echo "Bindung an NODE_OPERATOR_WALLET bleibt gesetzt: $([ -n "$(grep -E '^NODE_OPERATOR_BINDING_SIGNATURE=.' "$ENVF" || true)" ] && echo ja || echo nein)"
echo "Alte .env gesichert: /root/backups/validator-env-vor-neustart-$STAMP"
