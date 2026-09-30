# Neuer Signierschluessel fuer C1 beim Neustart bei null (Betreiber,
# 30.09.2026): RELAYER_PRIVATE_KEY war die persoenliche Wallet des
# Betreibers. C1 bekommt einen eigenen Schluessel, der NIE ausgegeben wird;
# ausgegeben wird nur seine Adresse. C1 ist danach der einzige Produzent
# (AUTHORIZED_VALIDATORS = diese Adresse). Die alte Bindung gilt nicht mehr --
# der Betreiber bindet C1 nach seiner Registrierung neu (/node-binding).
set -euo pipefail
[ "${BESTAETIGT:-}" = "ALLE GUTHABEN WEG" ] || { echo "ABBRUCH: keine Bestaetigung"; exit 1; }
ENVF=/root/Aequitas/deploy/validator/.env
[ -f "$ENVF" ] || { echo "ABBRUCH: $ENVF fehlt"; exit 1; }
laeuft "$NODE" && { echo "ABBRUCH: $NODE laeuft noch -- erst leeren"; exit 1; }
laeuft "$PROOF" || { echo "ABBRUCH: $PROOF laeuft nicht (erzeugt den Schluessel mit ethers)"; exit 1; }

# MPC mit fest eingetragenen Parteien kennt die Signieradressen. Mit nur C1
# und einem neuen Schluessel passt MPC_PEERS nicht mehr -- lieber anhalten
# als eine halbe Umstellung.
MPCP="$(grep -E '^MPC_PEERS=' "$ENVF" | tail -1 | cut -d= -f2- || true)"
[ -z "$MPCP" ] || { echo "ABBRUCH: MPC_PEERS ist in $ENVF gesetzt -- Umstellung erst nach Klaerung"; exit 1; }

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p /root/backups && chmod 700 /root/backups
install -m 600 "$ENVF" "/root/backups/validator-env-vor-neustart-$STAMP"

# Schluessel erzeugen: nur in dieser Variable, nie auf stdout des Workflows.
PAAR="$(docker exec "$PROOF" node -e "const {Wallet}=require('ethers');const w=Wallet.createRandom();process.stdout.write(w.privateKey+' '+w.address.toLowerCase())")"
PK="${PAAR%% *}"; ADDR="${PAAR##* }"
[[ "$PK" =~ ^0x[0-9a-f]{64}$ ]] && [[ "$ADDR" =~ ^0x[0-9a-f]{40}$ ]] || { echo "ABBRUCH: Schluessel unlesbar"; exit 1; }

setze() { # setze NAME WERT -- ersetzt die Zeile oder haengt sie an
  local tmp; tmp="$(mktemp)"
  grep -vE "^$1=" "$ENVF" > "$tmp" || true
  printf '%s=%s\n' "$1" "$2" >> "$tmp"
  install -m 600 "$tmp" "$ENVF"; rm -f "$tmp"
}
setze RELAYER_PRIVATE_KEY "$PK"
setze AUTHORIZED_VALIDATORS "$ADDR"
setze NODE_OPERATOR_BINDING_SIGNATURE ""
if grep -qE '^RELAYER_ADDRESS=' "$ENVF"; then setze RELAYER_ADDRESS "$ADDR"; fi
unset PK PAAR

echo "Neue Signieradresse von C1: $ADDR"
echo "AUTHORIZED_VALIDATORS = $ADDR (C1 allein)"
echo "Alte .env gesichert: /root/backups/validator-env-vor-neustart-$STAMP"
