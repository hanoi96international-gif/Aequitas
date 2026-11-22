#!/usr/bin/env bash
# Zweige aufraeumen: loescht nur Zweige, deren Inhalt nachweislich im
# Hauptzweig steht. Alles andere bleibt.
#
#   zweige_aufraeumen.sh pruefen   Liste ausgeben, nichts aendern (Standard)
#   zweige_aufraeumen.sh loeschen  dieselbe Liste loeschen
#
# Ein Zweig gilt als erledigt, wenn eine dieser Pruefungen zutrifft:
#   A  seine Spitze ist Vorfahre des Hauptzweigs (gemergt),
#   B  ein Merge in den Hauptzweig aendert nichts (Inhalt schon drin),
#   D  seine Spitze ist genau der Stand eines PRs #N, und im Hauptzweig
#      steht ein Commit, dessen Betreff mit "(#N)" endet (Squash-Merge).
# Nie geloescht werden: der Hauptzweig, Zweige mit offenem PR (Liste in
# $OFFENE_PR_ZWEIGE, ein Name je Zeile), Zweige mit einem Commit juenger
# als $MINDESTALTER_TAGE (Standard 2; laufende Sitzungen).
#
# Loeschen mit --force-with-lease auf die gepruefte Spitze: hat jemand den
# Zweig seitdem bewegt, schlaegt das Loeschen fuer ihn fehl (fail-closed).
# Jede geloeschte Spitze steht im Log (Wiederherstellen:
# git push origin <sha>:refs/heads/<zweig>).
#
# Erwartet einen Klon mit allen Zweigen unter refs/remotes/origin/* und
# den PR-Staenden unter refs/pr/* (refs/pull/*/head).
set -euo pipefail

MODUS="${1:-pruefen}"
case "$MODUS" in pruefen|loeschen) ;; *) echo "Modus: pruefen|loeschen" >&2; exit 2 ;; esac
HAUPT="${HAUPTZWEIG:?HAUPTZWEIG fehlt}"
ALTER="${MINDESTALTER_TAGE:-2}"
case "$ALTER" in ''|*[!0-9]*) echo "MINDESTALTER_TAGE: nur Ziffern" >&2; exit 2 ;; esac
OFFEN="${OFFENE_PR_ZWEIGE:-}"
HOECHSTENS="${HOECHSTENS:-500}"

D="origin/$HAUPT"
git rev-parse --verify -q "$D" >/dev/null || { echo "Hauptzweig $D fehlt" >&2; exit 1; }
DT="$(git rev-parse "$D^{tree}")"
GRENZE=$(( $(date +%s) - ALTER * 86400 ))
GEMERGT="$(git log "$D" --format=%s | sed -n 's/.*(#\([0-9][0-9]*\))$/\1/p' | sort -u)"

liste=()
while read -r sha ref; do
  z="${ref#refs/remotes/origin/}"
  if [ "$z" = "HEAD" ] || [ "$z" = "$HAUPT" ]; then continue; fi
  if [ -n "$OFFEN" ] && grep -qxF -- "$z" <<<"$OFFEN"; then
    echo "bleibt  $z (offener PR)"; continue
  fi
  if [ "$(git log -1 --format=%ct "$sha")" -gt "$GRENZE" ]; then
    echo "bleibt  $z (juenger als $ALTER Tage)"; continue
  fi
  grund=""
  if git merge-base --is-ancestor "$sha" "$D"; then
    grund="A gemergt"
  elif [ "$(git merge-tree --write-tree "$D" "$sha" 2>/dev/null | head -1)" = "$DT" ]; then
    grund="B Inhalt im Hauptzweig"
  else
    for pr in $(git for-each-ref --format='%(objectname) %(refname)' 'refs/pr/*' | awk -v s="$sha" '$1==s {sub("refs/pr/","",$2); print $2}'); do
      if grep -qx -- "$pr" <<<"$GEMERGT"; then grund="D Squash-PR #$pr"; break; fi
    done
  fi
  if [ -z "$grund" ]; then echo "bleibt  $z (eigener Inhalt)"; continue; fi
  echo "weg     $z $sha ($grund)"
  liste+=("$sha $z")
done < <(git for-each-ref --format='%(objectname) %(refname)' 'refs/remotes/origin/*')

echo "Zusammen: ${#liste[@]} Zweige erledigt."
[ "${#liste[@]}" -le "$HOECHSTENS" ] || { echo "Mehr als $HOECHSTENS -- abgebrochen, bitte pruefen." >&2; exit 1; }
[ "$MODUS" = loeschen ] || exit 0

fehler=0
for e in "${liste[@]}"; do
  sha="${e%% *}"; z="${e#* }"
  if git push -q origin --force-with-lease="refs/heads/$z:$sha" ":refs/heads/$z" 2>/dev/null; then
    echo "geloescht $sha $z"
  else
    echo "NICHT geloescht $z (bewegt oder Fehler)"; fehler=1
  fi
done
exit "$fehler"
