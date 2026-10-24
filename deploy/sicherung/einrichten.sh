#!/usr/bin/env bash
# Richtet auf C1 den Sicherungsschluessel ein (siehe sicherung-befehl.sh).
# Laeuft als root ueber .github/workflows/sicherung-einrichten.yml, also in
# der Umgebung `produktion` mit Freigabe. Aufruf:
#
#   [BIO_EMPFAENGER=age1...] bash einrichten.sh <Verzeichnis mit den Dateien>
#
# Das Verzeichnis enthaelt backup-ledger.sh, backup-biometrie.sh,
# sicherung-befehl.sh und sicherung_ed25519.pub aus dem Repo.
#
# Wiederholbar: ein zweiter Lauf ersetzt Skripte und Schluesselzeile, er
# haeuft nichts an. Ohne BIO_EMPFAENGER bleibt ein schon hinterlegter
# Empfaenger, wie er ist.
set -euo pipefail

QUELLE="${1:?Verzeichnis mit den Dateien angeben}"
LIB=/usr/local/lib/aequitas-sicherung
ETC=/etc/aequitas-sicherung
STAND=/var/lib/aequitas-sicherung
BEFEHL=/usr/local/sbin/aequitas-sicherung
AK=/root/.ssh/authorized_keys
KOMMENTAR=aequitas-sicherung

for f in backup-ledger.sh backup-biometrie.sh sicherung-befehl.sh sicherung_ed25519.pub; do
  [ -f "$QUELLE/$f" ] || { echo "::error::$f fehlt in $QUELLE"; exit 1; }
done
for f in backup-ledger.sh backup-biometrie.sh sicherung-befehl.sh; do
  bash -n "$QUELLE/$f" || { echo "::error::$f ist kein gueltiges Bash"; exit 1; }
done
command -v flock >/dev/null || { echo "::error::flock fehlt"; exit 1; }

# Oeffentlicher Schluessel: genau eine Zeile ssh-ed25519 <base64> [Kommentar].
PUB="$(cat "$QUELLE/sicherung_ed25519.pub")"
[ "$(printf '%s\n' "$PUB" | grep -c .)" = 1 ] || { echo "::error::Schluesseldatei hat nicht genau eine Zeile"; exit 1; }
printf '%s\n' "$PUB" | grep -Eq '^ssh-ed25519 [A-Za-z0-9+/]{68}( [A-Za-z0-9._@-]+)?$' \
  || { echo "::error::kein ed25519-Schluessel"; exit 1; }
B64="$(printf '%s\n' "$PUB" | awk '{print $2}')"

# Empfaenger der Biometrie-Sicherung (oeffentlich), nur wenn angegeben.
if [ -n "${BIO_EMPFAENGER:-}" ]; then
  R="$BIO_EMPFAENGER"; REST="${R#age1}"
  if [ "$REST" = "$R" ] || [ "${#R}" -ne 62 ] || case "$REST" in *[!qpzry9x8gf2tvdw0s3jn54khce6mua7l]*) true ;; *) false ;; esac; then
    echo "::error::BIO_EMPFAENGER ist kein oeffentlicher age-Schluessel (age1..., 62 Zeichen). Nie den privaten (AGE-SECRET-KEY-...) angeben."
    exit 1
  fi
fi

# 1. Skripte. Nur root darf sie aendern; der Schluessel fuehrt genau diese aus.
install -d -m 755 -o root -g root "$LIB"
install -d -m 700 -o root -g root "$ETC" "$STAND"
install -m 755 -o root -g root "$QUELLE/backup-ledger.sh" "$QUELLE/backup-biometrie.sh" "$LIB/"
install -m 755 -o root -g root "$QUELLE/sicherung-befehl.sh" "$BEFEHL"
if [ -n "${BIO_EMPFAENGER:-}" ]; then
  printf '%s\n' "$BIO_EMPFAENGER" > "$ETC/bio-empfaenger.neu"
  chmod 600 "$ETC/bio-empfaenger.neu"
  mv "$ETC/bio-empfaenger.neu" "$ETC/bio-empfaenger"
  echo "age-Empfaenger hinterlegt"
fi

# 2. Selbsttest des erzwungenen Befehls, BEVOR der Schluessel gilt.
selbst() { env -i PATH="$PATH" SSH_ORIGINAL_COMMAND="$1" "$BEFEHL" >/dev/null 2>&1; }
for boese in '' 'id' 'ledger; id' 'ledger && id' $'ledger\nid' 'ledger ' ' ledger' 'bash' 'ledger-datei /etc/shadow' '../ledger'; do
  rc=0; selbst "$boese" || rc=$?
  [ "$rc" = 2 ] || { echo "::error::Selbsttest: '$(printf '%q' "$boese")' nicht abgewiesen (rc=$rc) -- Schluessel wird NICHT eingetragen"; exit 1; }
done
selbst pruefsummen || { echo "::error::Selbsttest: pruefsummen scheitert -- Schluessel wird NICHT eingetragen"; exit 1; }
echo "Selbsttest: 10 Angriffe abgewiesen, pruefsummen ok"

# 3. Schluesselzeile. Vorher sichern; der Root-Zugang (alle anderen Zeilen)
# bleibt unveraendert -- sonst Abbruch, nichts ersetzt.
[ -s "$AK" ] || { echo "::error::$AK fehlt oder ist leer -- Abbruch"; exit 1; }
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
cp -p "$AK" "$AK.vor-sicherung-$STAMP"
ZEILE="restrict,command=\"$BEFEHL\" ssh-ed25519 $B64 $KOMMENTAR"
# Alte Zeilen dieses Zwecks weg -- und JEDE Zeile mit demselben Schluessel,
# damit er nirgends ohne die Beschraenkung steht.
{ grep -v -F " $KOMMENTAR" "$AK" | grep -v -F "$B64" || true; } > "$AK.neu"
VORHER="$(grep -v -F " $KOMMENTAR" "$AK" | grep -v -F "$B64" | grep -c . || true)"
NACHHER="$(grep -c . "$AK.neu" || true)"
[ "$VORHER" = "$NACHHER" ] && [ "$NACHHER" -ge 1 ] || { rm -f "$AK.neu"; echo "::error::Root-Schluessel wuerden sich aendern -- Abbruch"; exit 1; }
printf '%s\n' "$ZEILE" >> "$AK.neu"
chmod 600 "$AK.neu"
mv "$AK.neu" "$AK"
ls -1t "$AK".vor-sicherung-* 2>/dev/null | tail -n +4 | xargs -r rm -f
echo "Schluessel eingetragen: $(ssh-keygen -lf <(printf 'ssh-ed25519 %s\n' "$B64") | awk '{print $2}') (beschraenkt auf $BEFEHL), $NACHHER weitere Schluessel unveraendert"

# 4. Stand
env -i PATH="$PATH" SSH_ORIGINAL_COMMAND=pruefsummen "$BEFEHL"
