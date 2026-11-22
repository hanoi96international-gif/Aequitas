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
# Zusaetzlich (LISTE=<datei>): vom Betreiber entschiedene Zweige, je Zeile
# "<sha> <zweig>". Geloescht wird nur, wenn die Spitze noch genau <sha> ist
# (sonst "bewegt", bleibt); Hauptzweig und offene PRs bleiben auch hier.
# Mit BUNDLE=<datei> werden alle zu loeschenden Zweige vorher in ein
# Git-Bundle gesichert (fehlt es danach, wird nichts geloescht).
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
    for pr in $(git for-each-ref --format='%(objectname) %(refname)' 'refs/pr/' | awk -v s="$sha" '$1==s {sub("refs/pr/","",$2); print $2}'); do
      if grep -qx -- "$pr" <<<"$GEMERGT"; then grund="D Squash-PR #$pr"; break; fi
    done
  fi
  if [ -z "$grund" ]; then echo "bleibt  $z (eigener Inhalt)"; continue; fi
  echo "weg     $z $sha ($grund)"
  liste+=("$sha $z")
done < <(git for-each-ref --format='%(objectname) %(refname)' 'refs/remotes/origin/')

if [ -n "${LISTE:-}" ]; then
  [ -f "$LISTE" ] || { echo "LISTE $LISTE fehlt" >&2; exit 1; }
  while read -r soll z rest; do
    case "$soll" in ''|\#*) continue ;; esac
    [[ "$soll" =~ ^[0-9a-f]{40}$ ]] && [ -n "$z" ] && [ -z "$rest" ] || { echo "LISTE: ungueltige Zeile: $soll $z $rest" >&2; exit 1; }
    if [ "$z" = "$HAUPT" ]; then echo "bleibt  $z (Hauptzweig, auch auf der Liste)"; continue; fi
    if [ -n "$OFFEN" ] && grep -qxF -- "$z" <<<"$OFFEN"; then echo "bleibt  $z (offener PR, auch auf der Liste)"; continue; fi
    ist="$(git rev-parse -q --verify "refs/remotes/origin/$z" || true)"
    if [ -z "$ist" ]; then echo "fehlt   $z (schon weg)"; continue; fi
    if [ "$ist" != "$soll" ]; then echo "bleibt  $z (bewegt: $ist statt $soll)"; continue; fi
    doppelt=0; for e in "${liste[@]}"; do [ "${e#* }" = "$z" ] && doppelt=1; done
    [ "$doppelt" = 1 ] && continue
    echo "weg     $z $soll (L Liste des Betreibers)"
    liste+=("$soll $z")
  done < "$LISTE"
fi

echo "Zusammen: ${#liste[@]} Zweige erledigt."
[ "${#liste[@]}" -le "$HOECHSTENS" ] || { echo "Mehr als $HOECHSTENS -- abgebrochen, bitte pruefen." >&2; exit 1; }
[ "$MODUS" = loeschen ] || exit 0
[ "${#liste[@]}" -gt 0 ] || exit 0

if [ -n "${BUNDLE:-}" ]; then
  if [ "${NUR_SICHERN:-}" = 1 ]; then
    # Schritt 1: sichern, nichts loeschen (der Workflow laedt das Bundle hoch).
    refs=(); for e in "${liste[@]}"; do refs+=("refs/remotes/origin/${e#* }"); done
    git bundle create -q "$BUNDLE" "${refs[@]}" || { echo "Bundle fehlgeschlagen" >&2; exit 1; }
    git bundle verify -q "$BUNDLE" >/dev/null || { echo "Bundle ungueltig" >&2; exit 1; }
    echo "Sicherung: $BUNDLE ($(du -h "$BUNDLE" | cut -f1), ${#refs[@]} Zweige)"
    exit 0
  fi
  # Schritt 2: nur loeschen, was mit genau dieser Spitze im Bundle steckt.
  [ -f "$BUNDLE" ] || { echo "Keine Sicherung $BUNDLE -- nichts geloescht" >&2; exit 1; }
  git bundle verify -q "$BUNDLE" >/dev/null || { echo "Sicherung ungueltig -- nichts geloescht" >&2; exit 1; }
  koepfe="$(git bundle list-heads "$BUNDLE")"
  gesichert=()
  for e in "${liste[@]}"; do
    if grep -qxF -- "${e%% *} refs/remotes/origin/${e#* }" <<<"$koepfe"; then gesichert+=("$e"); else echo "bleibt  ${e#* } (nicht in der Sicherung)"; fi
  done
  liste=("${gesichert[@]}")
fi

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
