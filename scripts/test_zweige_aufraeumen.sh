#!/usr/bin/env bash
# Test fuer zweige_aufraeumen.sh mit echten Git-Repos (kein Netz).
# Gutfall: gemergt (A), Inhalt im Hauptzweig (B), Squash-PR (D) gehen weg.
# Missbrauch/Schutz: eigener Inhalt, offener PR, junger Zweig, ein PR-Stand
# ohne Merge, ein "(#N)" fuer einen anderen Stand und ein Zweig, der
# zwischen Pruefen und Loeschen bewegt wird, bleiben.
set -euo pipefail
SKRIPT="$(cd "$(dirname "$0")" && pwd)/zweige_aufraeumen.sh"
T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t
alt() { GIT_AUTHOR_DATE="2026-01-01T00:00:00Z" GIT_COMMITTER_DATE="2026-01-01T00:00:00Z" "$@"; }
fehl() { echo "FEHLER: $*" >&2; exit 1; }

git init -q --bare "$T/origin.git"
git init -q -b main "$T/w"; cd "$T/w"
git remote add origin "$T/origin.git"
echo a > a; git add a; alt git commit -qm start; git push -q origin main

neu() { git checkout -q -b "$1" main; echo "$2" > "$3"; git add "$3"; alt git commit -qm "$1"; git push -q origin "$1"; git checkout -q main; }

# A: normal gemergt
neu gemergt x x; git merge -q --ff-only gemergt
# Namen mit Schraegstrich (wie claude/...): gemergt und eigener Inhalt
neu claude/tief/gemergt t t; git merge -q --no-edit claude/tief/gemergt
neu claude/tief/eigen u u
# B: gleicher Inhalt per Rosinenpicken in main
neu inhalt y y; git checkout -q inhalt -- y; alt git commit -qm "y uebernommen"
# D: Squash-PR #7 (Zweigspitze = refs/pull/7/head)
neu squash z z; git push -q origin "squash:refs/pull/7/head"
git checkout -q squash -- z; alt git commit -qm "squash (#7)"
echo z2 > z; alt git commit -qam "z danach weiter geaendert"   # B greift nicht mehr, nur D
# PR #8 nur geoeffnet, nie gemergt (kein "(#8)" im Hauptzweig)
neu pr_offen_ohne_merge p p; git push -q origin "pr_offen_ohne_merge:refs/pull/8/head"
# "(#9)" steht in main, aber PR #9 zeigt auf einen anderen Stand als der Zweig
neu falscher_pr q q; git push -q origin "main:refs/pull/9/head"; echo r > r; git add r; alt git commit -qm "anderes (#9)"
# eigener Inhalt
neu eigen e e
# offener PR (Name in OFFENE_PR_ZWEIGE), Inhalt schon gemergt
neu offen o o; git merge -q --no-edit offen
# junger Zweig, Inhalt gemergt
git checkout -q -b jung main; echo j > j; git add j; git commit -qm jung; git push -q origin jung; git checkout -q main; git merge -q --no-edit jung
git push -q origin main

git fetch -q origin '+refs/heads/*:refs/remotes/origin/*' '+refs/pull/*/head:refs/pr/*'
export HAUPTZWEIG=main OFFENE_PR_ZWEIGE="offen"
aus="$("$SKRIPT" pruefen)"; echo "$aus"
for z in gemergt inhalt squash claude/tief/gemergt; do grep -q "^weg     $z " <<<"$aus" || fehl "$z sollte weg"; done
grep -q "^bleibt  claude/tief/eigen (eigener Inhalt)" <<<"$aus" || fehl "claude/tief/eigen muss bleiben"
grep -q "^weg     squash .*(D Squash-PR #7)" <<<"$aus" || fehl "squash nicht ueber D erkannt"
for z in eigen offen jung pr_offen_ohne_merge falscher_pr; do grep -q "^bleibt  $z " <<<"$aus" || fehl "$z muss bleiben"; done
grep -q "^weg     main" <<<"$aus" && fehl "Hauptzweig"
git ls-remote --heads origin | grep -q refs/heads/gemergt || fehl "pruefen hat geloescht"

# Ungueltige Eingaben
"$SKRIPT" irgendwas >/dev/null 2>&1 && fehl "falscher Modus angenommen"
MINDESTALTER_TAGE="1; rm" "$SKRIPT" pruefen >/dev/null 2>&1 && fehl "MINDESTALTER_TAGE nicht geprueft"
HOECHSTENS=2 "$SKRIPT" loeschen >/dev/null 2>&1 && fehl "Obergrenze nicht eingehalten"
git ls-remote --heads origin | grep -q refs/heads/gemergt || fehl "Obergrenze: trotzdem geloescht"

# Zweig zwischen Pruefen und Loeschen bewegen: Lease muss greifen
# (aus einem zweiten Klon; die lokale Sicht bleibt alt, kein fetch)
git clone -q "$T/origin.git" "$T/w2"
( cd "$T/w2" && git checkout -q gemergt && echo neu > n && git add n && git commit -qm neu && git push -q origin gemergt )
set +e; aus="$("$SKRIPT" loeschen)"; rc=$?; set -e; echo "$aus"
[ "$rc" -ne 0 ] || fehl "bewegter Zweig: Exit 0"
grep -q "^NICHT geloescht gemergt" <<<"$aus" || fehl "bewegter Zweig geloescht"
git ls-remote --heads origin | grep -q refs/heads/gemergt || fehl "bewegter Zweig fehlt"
for z in inhalt squash; do git ls-remote --heads origin | grep -q "refs/heads/$z$" && fehl "$z nicht geloescht"; done
for z in eigen offen jung main; do git ls-remote --heads origin | grep -q "refs/heads/$z$" || fehl "$z faelschlich geloescht"; done
grep -q "^geloescht [0-9a-f]\{40\} squash$" <<<"$aus" || fehl "Spitze nicht im Log"

# Liste des Betreibers: "eigen" (eigener Inhalt) nur mit passender Spitze,
# "falscher_pr" mit falscher Spitze bleibt, offener PR und Hauptzweig bleiben.
git fetch -q --prune origin '+refs/heads/*:refs/remotes/origin/*'
L="$T/liste.txt"
{ echo "# Kommentar"
  echo "$(git rev-parse origin/eigen) eigen"
  echo "$(git rev-parse origin/main) falscher_pr"
  echo "$(git rev-parse origin/offen) offen"
  echo "$(git rev-parse origin/main) main"; } > "$L"
aus="$(LISTE="$L" "$SKRIPT" pruefen)"; echo "$aus"
grep -q "^weg     eigen .*(L Liste" <<<"$aus" || fehl "Liste: eigen nicht vorgesehen"
grep -q "^bleibt  falscher_pr (bewegt" <<<"$aus" || fehl "Liste: falsche Spitze angenommen"
grep -q "^bleibt  offen (offener PR, auch auf der Liste)" <<<"$aus" || fehl "Liste: offener PR"
grep -q "^bleibt  main (Hauptzweig" <<<"$aus" || fehl "Liste: Hauptzweig"
echo "kaputt" > "$T/kaputt.txt"
LISTE="$T/kaputt.txt" "$SKRIPT" pruefen >/dev/null 2>&1 && fehl "Liste: ungueltige Zeile angenommen"
LISTE="$T/gibtsnicht" "$SKRIPT" pruefen >/dev/null 2>&1 && fehl "Liste: fehlende Datei angenommen"
eigen_sha="$(git rev-parse origin/eigen)"
# Ohne Sicherung, mit unschreibbarem Pfad oder mit einer Sicherung, in der
# der Zweig fehlt: nichts darf geloescht werden.
LISTE="$L" BUNDLE="$T/gibtsnicht/x.bundle" NUR_SICHERN=1 "$SKRIPT" loeschen >/dev/null 2>&1 && fehl "Bundle-Fehler ignoriert"
LISTE="$L" BUNDLE="$T/gibtsnicht/x.bundle" "$SKRIPT" loeschen >/dev/null 2>&1 && fehl "ohne Sicherung geloescht"
git bundle create -q "$T/leer.bundle" refs/remotes/origin/main
aus="$(LISTE="$L" BUNDLE="$T/leer.bundle" "$SKRIPT" loeschen)" || true
grep -q "^bleibt  eigen (nicht in der Sicherung)" <<<"$aus" || fehl "nicht gesicherter Zweig nicht erkannt"
git ls-remote --heads origin | grep -q "refs/heads/eigen$" || fehl "nicht gesicherter Zweig geloescht"
LISTE="$L" BUNDLE="$T/s.bundle" NUR_SICHERN=1 "$SKRIPT" loeschen >/dev/null
git ls-remote --heads origin | grep -q "refs/heads/eigen$" || fehl "Sichern hat geloescht"
aus="$(LISTE="$L" BUNDLE="$T/s.bundle" "$SKRIPT" loeschen)" || true; echo "$aus"
git ls-remote --heads origin | grep -q "refs/heads/eigen$" && fehl "Liste: eigen nicht geloescht"
git ls-remote --heads origin | grep -q "refs/heads/falscher_pr$" || fehl "Liste: falscher_pr geloescht"
git bundle list-heads "$T/s.bundle" | grep -q "^$eigen_sha refs/remotes/origin/eigen$" || fehl "eigen nicht im Bundle"
git init -q --bare "$T/zurueck.git"; git -C "$T/zurueck.git" fetch -q "$T/s.bundle" "refs/remotes/origin/eigen:refs/heads/eigen"
[ "$(git -C "$T/zurueck.git" rev-parse eigen)" = "$eigen_sha" ] || fehl "aus dem Bundle nicht wiederherstellbar"
echo "OK zweige_aufraeumen"
