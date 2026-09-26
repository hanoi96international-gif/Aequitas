#!/usr/bin/env bash
# Wachhund auf einer Box einrichten: /api/wache lokal und beim Partner alle
# 5 min, Meldung ueber Telegram und/oder ntfy. Laeuft ueber
# .github/workflows/wachhund-telegram-installieren.yml; TOKEN, CHAT,
# NTFY_KANAL, PARTNER, BOXNAME und TESTEN kommen als export-Zeilen vor dem
# Skript ueber stdin (nie auf der Befehlszeile, die ps zeigt).
set -euo pipefail
NTFY_KANAL="${NTFY_KANAL:-}"
mkdir -p /root/wachhund
umask 077
printf 'TELEGRAM_BOT_TOKEN=%s\nTELEGRAM_CHAT_ID=%s\nNTFY_KANAL=%s\nPARTNER=%s\nBOXNAME=%s\n' "$TOKEN" "$CHAT" "$NTFY_KANAL" "$PARTNER" "$BOXNAME" > /root/wachhund/env
chmod 600 /root/wachhund/env
cat > /root/wachhund/wachhund.sh <<'SKRIPT'
#!/bin/bash
# Wachhund: /api/wache lokal + Partner -> Telegram und/oder ntfy bei Umschlag, alle 6 h bei Rot, einmal bei Gruen.
set -u
TELEGRAM_BOT_TOKEN=""; TELEGRAM_CHAT_ID=""; NTFY_KANAL=""
. /root/wachhund/env
STATE=/root/wachhund/state
LOG=/root/wachhund/wachhund.log
jetzt=$(date +%s)
rot=""
pruefe() { # name url
  local body code
  body=$(curl -s --max-time 20 -w '\n%{http_code}' "$2" 2>/dev/null || true)
  code=$(printf '%s' "$body" | tail -n1)
  body=$(printf '%s' "$body" | sed '$d')
  if [ "$code" = "200" ]; then return 0; fi
  if [ "$code" = "503" ]; then
    local befunde
    befunde=$(printf '%s' "$body" | python3 -c "import sys,json; d=json.load(sys.stdin); print(' | '.join(d.get('befunde') or []))" 2>/dev/null || echo "(unlesbar)")
    rot="${rot}${1}: ${befunde}"$'\n'
  else
    rot="${rot}${1}: keine Antwort (HTTP ${code:-0})"$'\n'
  fi
}
pruefe "$BOXNAME" "http://127.0.0.1:8080/api/wache"
pruefe "partner $PARTNER" "http://$PARTNER:8080/api/wache"
vorher_status=gruen; vorher_alarm=0
if [ -f "$STATE" ]; then . "$STATE"; fi
senden() {
  if [ -n "$TELEGRAM_BOT_TOKEN" ] && [ -n "$TELEGRAM_CHAT_ID" ]; then
    curl -s --max-time 20 -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
      --data-urlencode "chat_id=${TELEGRAM_CHAT_ID}" --data-urlencode "text=$1" >/dev/null 2>&1 || true
  fi
  if [ -n "$NTFY_KANAL" ]; then
    curl -s --max-time 20 -H "Title: Aequitas" -H "Priority: high" \
      --data-binary "$1" "https://ntfy.sh/${NTFY_KANAL}" >/dev/null 2>&1 || true
  fi
}
if [ -n "$rot" ]; then
  if [ "$vorher_status" != "rot" ] || [ $((jetzt - vorher_alarm)) -ge 21600 ]; then
    senden "🔴 Aequitas ($BOXNAME): $(printf '%s' "$rot")"
    vorher_alarm=$jetzt
    echo "$(date -Is) ROT gemeldet: $(printf '%s' "$rot" | tr '\n' ' ')" >> "$LOG"
  fi
  printf 'vorher_status=rot\nvorher_alarm=%s\n' "$vorher_alarm" > "$STATE"
else
  if [ "$vorher_status" = "rot" ]; then
    senden "🟢 Aequitas ($BOXNAME): wieder gruen -- beide Knoten antworten, Tor zu, Coordinatoren da."
    echo "$(date -Is) wieder GRUEN" >> "$LOG"
  fi
  printf 'vorher_status=gruen\nvorher_alarm=%s\n' "$vorher_alarm" > "$STATE"
fi
SKRIPT
chmod 700 /root/wachhund/wachhund.sh
# Cron-Eintrag (idempotent).
( crontab -l 2>/dev/null | grep -v '/root/wachhund/wachhund.sh' ; echo '*/5 * * * * /root/wachhund/wachhund.sh >/dev/null 2>&1' ) | crontab -
echo "Cron: $(crontab -l | grep -c wachhund) Eintrag, Skript $(stat -c %a /root/wachhund/wachhund.sh), Env $(stat -c %a /root/wachhund/env)"
# Einmal jetzt laufen lassen (setzt den Zustand).
/root/wachhund/wachhund.sh
echo "Zustand: $(cat /root/wachhund/state | tr '\n' ' ')"
if [ "$TESTEN" = "true" ]; then
  TEXT="✅ Aequitas Wachhund auf ${BOXNAME} eingerichtet -- prueft alle 5 Minuten /api/wache hier und beim Partner."
  if [ -n "$TOKEN" ] && [ -n "$CHAT" ]; then
    curl -s --max-time 20 -X POST "https://api.telegram.org/bot${TOKEN}/sendMessage" \
      --data-urlencode "chat_id=${CHAT}" --data-urlencode "text=${TEXT}" \
      | grep -q '"ok":true' && echo "Telegram: Testnachricht angekommen" || { echo "FEHLER: Telegram hat die Testnachricht nicht angenommen (Token/Chat-ID pruefen)"; exit 1; }
  fi
  if [ -n "$NTFY_KANAL" ]; then
    # ntfy antwortet mit dem gespeicherten Ereignis, darin "id". Ohne
    # "id" ist nichts angekommen -- auch wenn curl selbst 0 liefert.
    curl -s --max-time 20 -H "Title: Aequitas" --data-binary "$TEXT" "https://ntfy.sh/${NTFY_KANAL}" \
      | grep -q '"id"' && echo "ntfy: Testnachricht angekommen" || { echo "FEHLER: ntfy.sh hat die Testnachricht nicht angenommen"; exit 1; }
  fi
fi
