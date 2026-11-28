#!/usr/bin/env bash
# Selbsttest eines Aequitas-Knotens (docs/KNOTEN_EINRICHTEN.md) -- EIN Befehl,
# der sagt, ob der Knoten bereit ist, und was noch fehlt.
#
#   Auf dem Server:      cd Aequitas/deploy/knoten && bash pruefen.sh
#   Von ueberall (z. B.  bash pruefen.sh 203.0.113.10
#   der erste Betreiber  vor seiner Zustimmung, docs/ZWEITER_KNOTEN_GESUCHT.md,
#   Checkliste Schritt 3)
#
# Geprueft wird nur LESEND (GET, kurze Fristen), nichts wird veraendert:
#   1. das Netz antwortet (Hoehe des Hauptknotens),
#   2. der Knoten antwortet und ist auf Stand (hoechstens SPIELRAUM Bloecke
#      hinter dem Netz; mehr = holt noch auf),
#   3. er ist an einen Menschen gebunden (Bestaetigung in der App, Tab "Knoten"),
#   4. sein Verifier antwortet ueber HTTPS (https://<IP-mit-Strichen>.sslip.io/health),
#   5. nur auf dem Server: die Selbstpruefung des Knotens (/api/wache = 200).
# Was der Knoten meldet, wird streng gelesen (Hoehe nur Ziffern, Adresse nur
# 0x + 40 Hex); was nicht passt, zaehlt als FEHLER, nie als ok.
#
# Ergebnis: 0 = bereit, 2 = laeuft, aber noch nicht fertig (holt auf, nicht
# gebunden), 1 = Fehler.
set -uo pipefail

NETZ="${AEQUITAS_NETZ:-https://aequitas.digital}"
SPIELRAUM=30

gruen() { printf '  \033[32mok\033[0m      %s\n' "$*"; }
gelb()  { printf '  \033[33mnoch\033[0m    %s\n' "$*"; WARN=1; }
rot()   { printf '  \033[31mFEHLER\033[0m  %s\n' "$*"; FEHLER=1; }
FEHLER=0; WARN=0

hole() { curl -fsS -m 10 "$1" 2>/dev/null || true; }
feld_zahl() { printf '%s' "$1" | grep -oE "\"$2\": ?[0-9]{1,12}[,}]" | head -1 | grep -oE '[0-9]{1,12}' || true; }
feld_adresse() { printf '%s' "$1" | grep -oE "\"$2\": ?\"0x[0-9a-fA-F]{40}\"" | head -1 | grep -oE '0x[0-9a-fA-F]{40}' | tr 'A-F' 'a-f' || true; }

ZIEL="${1:-}"
if [ -n "$ZIEL" ]; then
  [[ "$ZIEL" =~ ^([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})\.([0-9]{1,3})$ ]] || { echo "Keine IPv4 / not an IPv4: $ZIEL"; exit 1; }
  for teil in "${BASH_REMATCH[@]:1}"; do [ "$teil" -le 255 ] || { echo "Keine IPv4 / not an IPv4: $ZIEL"; exit 1; }; done
  KNOTEN="http://$ZIEL:8080"
  HOST="$(printf '%s' "$ZIEL" | tr '.' '-').sslip.io"
  LOKAL=0
  echo "Pruefe den Knoten $ZIEL von aussen / checking node $ZIEL from outside"
else
  KNOTEN="http://127.0.0.1:8080"
  HOST="$(grep -E '^VERIFIER_HOST=' ../verifier/.env 2>/dev/null | head -1 | cut -d= -f2- || true)"
  LOKAL=1
  echo "Pruefe diesen Knoten / checking this node"
fi

# 1. Netz
N="$(hole "$NETZ/api/status")"
H_NETZ="$(feld_zahl "$N" height)"
if [ -n "$H_NETZ" ]; then gruen "Netz antwortet, Hoehe $H_NETZ / network answers"; else rot "Netz $NETZ antwortet nicht -- spaeter noch einmal / network not reachable, try later"; fi

# 2. Knoten
S="$(hole "$KNOTEN/api/status")"
H="$(feld_zahl "$S" height)"
ADDR="$(feld_adresse "$S" validator_address)"
if [ -z "$H" ]; then
  if [ "$LOKAL" = 1 ]; then rot "Knoten antwortet nicht. Log: cd ../validator && docker compose logs --tail 50 node"
  else rot "Knoten antwortet nicht auf $KNOTEN (Port 8080 beim Anbieter offen?)"; fi
elif [ -n "$H_NETZ" ]; then
  ABSTAND=$(( H_NETZ - H ))
  if [ "$ABSTAND" -lt "-$SPIELRAUM" ]; then rot "Knoten meldet $(( -ABSTAND )) Bloecke MEHR als das Netz (Hoehe $H) -- anderer Zweig oder falsche Kette?"
  elif [ "$ABSTAND" -le "$SPIELRAUM" ]; then gruen "Knoten auf Stand, Hoehe $H / node in sync"
  else gelb "Knoten holt auf: $ABSTAND Bloecke hinter dem Netz (Hoehe $H) / catching up"; fi
else
  gelb "Knoten antwortet (Hoehe $H), Vergleich mit dem Netz nicht moeglich"
fi

# 3. Bindung an den Menschen
if [ -z "$ADDR" ]; then
  [ -n "$H" ] && rot "Knoten nennt keine Signieradresse (validator_address) / no signing address"
else
  B="$(hole "$NETZ/api/validator-binding?signing_address=$ADDR")"
  W="$(feld_adresse "$B" human_wallet)"
  if [ -n "$W" ]; then gruen "an den Menschen $W gebunden (Signieradresse $ADDR)"
  else gelb "noch nicht gebunden: in der App Tab \"Knoten\" die Kontrollzahl bestaetigen (Signieradresse $ADDR)"; fi
fi

# 4. Verifier
if ! [[ "$HOST" =~ ^[0-9]{1,3}(-[0-9]{1,3}){3}\.sslip\.io$ ]]; then
  rot "Verifier-Adresse unbekannt (deploy/verifier/.env, VERIFIER_HOST) -- Verifier eingerichtet? bash einrichten.sh"
elif [ -n "$(hole "https://$HOST/health")" ]; then
  gruen "Verifier antwortet: https://$HOST"
else
  rot "Verifier antwortet nicht: https://$HOST/health (Ports 80/443 offen? cd ../verifier && docker compose logs --tail 50)"
fi

# 5. Selbstpruefung (nur auf dem Server; /api/wache ist nicht fuer aussen gedacht)
if [ "$LOKAL" = 1 ] && [ -n "$H" ]; then
  CODE="$(curl -s -o /dev/null -w '%{http_code}' -m 10 "$KNOTEN/api/wache" 2>/dev/null || true)"
  if [ "$CODE" = 200 ]; then gruen "Selbstpruefung des Knotens bestanden (/api/wache)"
  else rot "Selbstpruefung meldet ein Problem (/api/wache, HTTP $CODE): curl -s localhost:8080/api/wache"; fi
fi

echo
if [ "$FEHLER" = 1 ]; then
  echo "Noch nicht bereit -- die FEHLER oben beheben, dann noch einmal pruefen. / Not ready yet."
  exit 1
elif [ "$WARN" = 1 ]; then
  echo "Laeuft, aber noch nicht fertig (siehe \"noch\"). In ein paar Minuten noch einmal. / Running, not finished yet."
  exit 2
fi
echo "Bereit. Naechster Schritt in der App, Karte \"Pruefer\": Schluessel freigeben, Bewerben. / Ready."
exit 0
