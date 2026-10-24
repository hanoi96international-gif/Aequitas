#!/usr/bin/env bash
# Erzwungener Befehl des Sicherungsschluessels auf C1.
#
# WARUM (24.10.2026). Seit `produktion` eine Pflicht-Freigabe hat, warteten
# auch die naechtlichen Sicherungen auf einen Klick -- vergessen hiess: keine
# Sicherung. Die Sicherungs-Workflows laufen deshalb in der Umgebung
# `sicherung` ohne Freigabe, mit einem EIGENEN Schluessel. Damit der nicht zum
# Root-Zugang ohne Freigabe wird, steht er in /root/.ssh/authorized_keys als
#
#   restrict,command="/usr/local/sbin/aequitas-sicherung" ssh-ed25519 ... aequitas-sicherung
#
# sshd fuehrt dann immer DIESES Skript aus, egal was der Aufrufer verlangt;
# was er verlangt, steht nur in SSH_ORIGINAL_COMMAND. `restrict` nimmt
# Terminal, Port-, Agent- und X11-Weiterleitung.
#
# WAS DER SCHLUESSEL KANN -- UND NUR DAS:
#   ledger, ledger-datei, ledger-zusammenfassung
#   bio, bio-datei, bio-zusammenfassung
#   pruefsummen
# Ausgefuehrt wird nur, was auf C1 installiert ist (sicherung-einrichten.yml),
# nie Code vom Aufrufer. Der age-Empfaenger der Biometrie-Sicherung liegt
# fest auf C1: wer den Schluessel stiehlt, kann sich die Daten nicht an
# seinen eigenen Schluessel verschluesseln lassen. Er bekommt hoechstens, was
# ohnehin als Artefakt im oeffentlichen Repo liegt (Zustand ohne
# biometrische Zeilen) und age-Chiffrat.
#
# GRENZEN. Ein Lauf je Art zugleich (flock), hoechstens einer je 10 Minuten
# (auch ein gescheiterter zaehlt), Laufzeit begrenzt (timeout), Dateien nur
# bis 500 MB.
set -uo pipefail
umask 077

LIB=/usr/local/lib/aequitas-sicherung
ETC=/etc/aequitas-sicherung
STAND=/var/lib/aequitas-sicherung
MIN_ABSTAND=600
MAX_DATEI=$((500 * 1024 * 1024))

BEFEHL="${SSH_ORIGINAL_COMMAND:-}"

protokoll() { logger -t aequitas-sicherung -- "$*" 2>/dev/null || true; }

ablehnen() {
  # Der verlangte Befehl kommt vom Aufrufer: nur gekuerzt und maskiert ins
  # Protokoll, nie zurueck in eine Shell.
  protokoll "abgewiesen: $(printf '%q' "$BEFEHL" | head -c 200)"
  echo "Befehl nicht erlaubt. Erlaubt: ledger, ledger-datei, ledger-zusammenfassung, bio, bio-datei, bio-zusammenfassung, pruefsummen" >&2
  exit 2
}

# Gueltiger oeffentlicher age-Schluessel: age1 + 58 Zeichen Bech32.
age_empfaenger_ok() {
  local r="$1" rest="${1#age1}"
  [ "$rest" != "$r" ] && [ "${#r}" -eq 62 ] && case "$rest" in *[!qpzry9x8gf2tvdw0s3jn54khce6mua7l]*) false ;; *) true ;; esac
}

# Ein Lauf je Art zugleich, hoechstens einer je MIN_ABSTAND Sekunden.
sperren() {
  local art="$1"
  mkdir -p "$STAND" && chmod 700 "$STAND"
  exec 9>"$STAND/$art.lock"
  flock -n 9 || { echo "Eine Sicherung ($art) laeuft schon." >&2; exit 3; }
  local zuletzt jetzt
  zuletzt="$(stat -c %Y "$STAND/$art.zuletzt" 2>/dev/null || echo 0)"
  jetzt="$(date +%s)"
  if [ $((jetzt - zuletzt)) -lt "$MIN_ABSTAND" ]; then
    echo "Letzte Sicherung ($art) vor weniger als $((MIN_ABSTAND / 60)) Minuten -- abgewiesen." >&2
    exit 4
  fi
  touch "$STAND/$art.zuletzt"
}

# Nie eine halb geschriebene Datei: waehrend eines Laufs derselben Art nichts.
datei_ausgeben() {
  local art="$1" f="$2" groesse
  mkdir -p "$STAND" && chmod 700 "$STAND"
  exec 9>"$STAND/$art.lock"
  flock -n 9 || { echo "Eine Sicherung ($art) laeuft gerade -- Datei spaeter abholen." >&2; exit 3; }
  [ -f "$f" ] || { echo "Keine Datei: $f" >&2; exit 1; }
  groesse="$(stat -c %s "$f")"
  [ "$groesse" -le "$MAX_DATEI" ] || { echo "Datei groesser als 500 MB -- nicht ausgegeben." >&2; exit 1; }
  exec cat -- "$f"
}

case "$BEFEHL" in
  ledger)
    sperren ledger
    protokoll "ledger: Start"
    BOX=c1 timeout 900 bash "$LIB/backup-ledger.sh"
    rc=$?
    protokoll "ledger: Ende ($rc)"
    exit "$rc"
    ;;
  ledger-datei)
    datei_ausgeben ledger /root/backup-latest.dump
    ;;
  ledger-zusammenfassung)
    datei_ausgeben ledger /root/backup-latest.summary
    ;;
  bio)
    R="$(head -n 1 "$ETC/bio-empfaenger" 2>/dev/null || true)"
    if ! age_empfaenger_ok "$R"; then
      echo "Kein gueltiger age-Empfaenger auf C1 hinterlegt -- nichts gesichert. sicherung-einrichten.yml mit bio_empfaenger starten (docs/BIOMETRIE_SICHERUNG.md)." >&2
      exit 5
    fi
    sperren bio
    protokoll "bio: Start"
    BIO_BACKUP_AGE_RECIPIENT="$R" timeout 1200 bash "$LIB/backup-biometrie.sh"
    rc=$?
    protokoll "bio: Ende ($rc)"
    exit "$rc"
    ;;
  bio-datei)
    datei_ausgeben bio /root/bio-sicherung/bio-latest.tar.age
    ;;
  bio-zusammenfassung)
    datei_ausgeben bio /root/bio-sicherung/bio-latest.summary
    ;;
  pruefsummen)
    ( cd "$LIB" && sha256sum backup-ledger.sh backup-biometrie.sh ) || exit 1
    if age_empfaenger_ok "$(head -n 1 "$ETC/bio-empfaenger" 2>/dev/null || true)"; then
      echo "bio_empfaenger=gesetzt"
    else
      echo "bio_empfaenger=fehlt"
    fi
    ;;
  *)
    ablehnen
    ;;
esac
