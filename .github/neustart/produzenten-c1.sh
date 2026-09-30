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
# Einziger Validator: keine Seeds (PRIMARY_NODE_URLS=keine, erprobt seit dem
# 24.09.2026 auf C2 allein). Sonst wartet C1 auf den ausgeschalteten C2: das
# Tor der taeglichen Verteilung bliebe zu ("never synced from any peer"),
# Selbstheilung und Totmann-Schalter prueften gegen eine tote Adresse, und ein
# gesetztes BOOTSTRAP_SNAPSHOT_URL wuerde beim leeren Start einen Snapshot
# holen wollen. Die alten Zeilen bleiben auskommentiert stehen, damit C2 nach
# der Neueinrichtung wieder eingetragen werden kann.
tmp="$(mktemp)"
sed -E 's/^(AUTHORIZED_VALIDATORS|PRIMARY_NODE_URLS|PRIMARY_NODE_URL|PEER_NODES|BOOTSTRAP_SNAPSHOT_URL|BOOTSTRAP_SIGNER)=/# vor Neustart 30.09.: &/' "$ENVF" > "$tmp"
printf 'AUTHORIZED_VALIDATORS=%s\nPRIMARY_NODE_URLS=keine\n' "$ADDR" >> "$tmp"
install -m 600 "$tmp" "$ENVF"; rm -f "$tmp"
echo "Seeds: PRIMARY_NODE_URLS=keine (einziger Validator); auskommentiert: $(grep -cE '^# vor Neustart 30.09.: ' "$ENVF") alte Zeilen"

echo "Signieradresse von C1 (bleibt): $ADDR"
echo "AUTHORIZED_VALIDATORS = $ADDR (C1 allein)"
echo "Bindung an NODE_OPERATOR_WALLET bleibt gesetzt: $([ -n "$(grep -E '^NODE_OPERATOR_BINDING_SIGNATURE=.' "$ENVF" || true)" ] && echo ja || echo nein)"
echo "Alte .env gesichert: /root/backups/validator-env-vor-neustart-$STAMP"
