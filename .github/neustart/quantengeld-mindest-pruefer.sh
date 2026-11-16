#!/usr/bin/env bash
# Haelt den Deploy an, wenn sich die Mindestzahl der Pruefer fuer eine
# Quantengeld-Freischaltung (quantenFreischaltungMindestPruefer,
# x/humanity/keeper/quanten_geld.go) gegenueber dem laufenden Knoten aendert
# und dort schon ein Satz eingetragen ist. Ein Satz, der unter der alten Regel
# beschlossen wurde (etwa mit der Stimme eines einzigen Pruefers), waere beim
# Nachspielen unter der neuen Regel ungueltig -- ein neu synchronisierender
# Knoten bliebe an diesem Block haengen. Ein Knoten ohne
# quanten.mindest_pruefer (aelterer Stand) zaehlt als "keine Mindestzahl".
# Ohne Antwort, mit kaputtem JSON oder unlesbarem Stand: anhalten.
# Aufruf: quantengeld-mindest-pruefer.sh URL
set -euo pipefail
BASE=$1
REPO=$(sed -n 's/^\tquantenFreischaltungMindestPruefer = \([0-9]\{1,3\}\)$/\1/p' x/humanity/keeper/quanten_geld.go)
[[ "$REPO" =~ ^[1-9][0-9]{0,2}$ ]] || { echo "::error::quantenFreischaltungMindestPruefer im Repo nicht lesbar"; exit 1; }
STATUS=$(curl -sf -m 15 "$BASE/api/status") || { echo "::error::laufender Knoten antwortet nicht -- Deploy angehalten"; exit 1; }
LIVE=$(printf '%s' "$STATUS" | jq -r '(.quanten // {}) | (.mindest_pruefer // 0) | tostring' 2>/dev/null) || { echo "::error::Antwort des laufenden Knotens ist kein JSON -- Deploy angehalten"; exit 1; }
[[ "$LIVE" =~ ^(0|[1-9][0-9]{0,2})$ ]] || { echo "::error::Mindestzahl im laufenden Knoten unlesbar: $LIVE"; exit 1; }
echo "Mindestzahl der Pruefer fuer eine Freischaltung: Repo $REPO, laufend $LIVE"
if [ "$REPO" = "$LIVE" ]; then
  echo "unveraendert"
  exit 0
fi
STAND=$(curl -sf -m 15 "$BASE/api/quanten/stand") || { echo "::error::/api/quanten/stand antwortet nicht -- Deploy angehalten"; exit 1; }
SAETZE=$(printf '%s' "$STAND" | jq -r 'if (.saetze | type) == "array" then (.saetze | length | tostring) else "x" end' 2>/dev/null) || { echo "::error::/api/quanten/stand ist kein JSON -- Deploy angehalten"; exit 1; }
[[ "$SAETZE" =~ ^[0-9]+$ ]] || { echo "::error::Saetze im Quantengeld-Stand unlesbar -- Deploy angehalten"; exit 1; }
if [ "$SAETZE" != 0 ]; then
  echo "::error::Im laufenden Knoten sind schon $SAETZE Quantengeld-Saetze eingetragen. Mit der neuen Mindestzahl ($REPO statt $LIVE) koennte ihre Freischaltung beim Nachspielen ungueltig werden. Erst die Stimmen jedes Satzes pruefen, dann ausrollen."
  exit 1
fi
echo "kein Satz eingetragen -- neue Mindestzahl ausrollen erlaubt"
