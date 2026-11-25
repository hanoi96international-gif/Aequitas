# Pilot Weg B – Quantengeld mit Handys als Präparatorinnen

Stand 25.11.2026. Diese Anleitung ist für den Betreiber und die Menschen im Pilot (B5).
Technischer Hintergrund: `forschung/quantengeld/DEZENTRAL.md` (Abschnitte B2b, B3, B4b).

## Was vorher passieren muss (Betreiber)

1. **App bauen** (von Hand): Actions → `build-apk.yml` → „Run workflow“, Variante `release`.
   Vorher in den Repo-Variablen der App die `EXPO_PUBLIC_*_FALLBACKS` prüfen: Es gibt zurzeit
   **keinen zweiten Knoten** (proof2 ist seit 03.10. abgeschaltet, Launch-Checkliste Punkt 16).
   Steht dort noch proof2, leeren. Die App filtert proof2 zwar selbst heraus, aber die Variable
   soll nichts Falsches behaupten.
2. **Stichtag Weg B: 26.11.2026, 10:00 Uhr MEZ (09:00 UTC)**, gesetzt am 25.11. (`quantenSWegBAbUnix` = 1795683600).
   Zum Verfahren:
   Der Betreiber nennt Datum und Uhrzeit (UTC). Dann kommt ein PR, der den Stichtag setzt.
   Er gilt als Konsensänderung und bekommt eine eigene Sicherheitsprüfung. Nach dem Merge läuft
   der Deploy (`deploy-c1.yml`) mit Freigabe durch den Betreiber.
   Der Deploy-Wächter verhindert einen rückwirkenden Stichtag.
   Empfehlung: Stichtag frühestens 2 bis 3 Tage nach dem App-Release, damit alle die App haben.
3. **Menschen finden** (siehe Rollen). Alle brauchen die App mit ihrer Wallet und müssen als Mensch
   registriert sein.

## Rollen

| Rolle | Wie viele | Was sie brauchen |
|---|---|---|
| Präparatorin | mindestens 7 im Los, besser 9–10 gemeldet (Ausfälle) | App, Menschen-Wallet, **eigenes IBM-Quantum-Konto** (kostenlos: 10 Minuten je 28 Tage) mit API-Schlüssel und Instanz (CRN) |
| Ausgeber | 1 | App, Menschen-Wallet, AEQ für Deckung + Pfand (Pfand = 10 % der Deckung; die App nennt beides vor dem Bestellen) |
| Inhaberin | 1 | App |
| Annahmestellen | 2 (Stelle 0 und Stelle 1) | App; zwei verschiedene Konten, beide nicht die Inhaberin |

Der Ausgeber kommt nie in sein eigenes Los (Kettenregel). Ist er selbst als Präparatorin gemeldet,
braucht es also eine Gemeldete mehr.

## Ablauf

**Tag 0 – nach dem Stichtag.** Präparatorinnen: Quantengeld-Seite → „Als Präparatorin melden“.
Danach in derselben Karte IBM-API-Schlüssel und CRN eingeben und „Zugang sichern“. Der Schlüssel
bleibt nur im Handy und wird nie wieder angezeigt. Im Los ist man erst **einen Tag nach** dem Melden.

**Tag 1 oder später – eine Ausgabe** (alles an einem Tag; der Auftrag gilt 1 Tag, die Bestellung 2 Tage):

1. **Ausgeber**: Quantengeld → „Quanten-Token ausgeben (Weg B)“ → v = 7, t = 4, Deckung → „Bestellen“.
   Nach gut 10 Minuten „Stand prüfen und losen“, bis der Stand „gelost“ ist. Dann „K kopieren“
   und K der Inhaberin schicken (Nachricht; K ist nicht geheim).
2. **Inhaberin**: „Quanten-Token empfangen“ → Schalter „Weg B“ an → K einfügen, n = 2822 lassen,
   die beiden Annahmestellen eintragen → „Bestellung anlegen“. Die Bestellung (Text, etwa 12 KB) kopieren
   und dem Ausgeber schicken.
3. **Ausgeber**: Bestellung der Inhaberin einfügen → „Gelosten beauftragen“.
4. **Präparatorinnen**: „Briefkasten prüfen und antworten“. Die Messung bei IBM kann dauern.
   Die App darf man schließen; ein späteres „Briefkasten prüfen“ setzt dieselbe Messung fort.
5. **Ausgeber**: „Briefkasten prüfen“, bis keine Antwort mehr fehlt. Dann „Bestätigungen anfordern“.
6. **Präparatorinnen**: noch einmal „Briefkasten prüfen und antworten“ (bestätigen).
7. **Ausgeber**: „Briefkasten prüfen“, bis alle Bestätigungen da sind. Dann „Ausgeben“
   (bestätigen) und „Paket an die Inhaberin senden“.
8. **Inhaberin**: „Paket abholen und prüfen“. Danach zum Einlösen Stelle 0 oder 1 wählen und den
   QR-Code **nur dieser Stelle** zeigen. Er wirkt dort wie Bargeld.
9. **Annahmestelle**: „Vorlage aus Weg B scannen“. Die App fordert die Öffnungen bei den Gelosten an.
10. **Präparatorinnen**: „Briefkasten prüfen und antworten“. Die Öffnung geht erst hinaus, wenn
    Bindung und Wahl auf der Kette final sind; also unter Umständen später noch einmal.
11. **Annahmestelle**: „Öffnungen sammeln“, dann „Annehmen und einlösen“. Fehlt eine Geloste,
    ersetzt das Paket des Ausgebers ihren Teil (R9).

## Wenn etwas hängt

- **„Antworten fehlen noch von: …“ beim Ausgeber**: Diese Präparatorinnen anschreiben, damit sie
  „Briefkasten prüfen“ tippen. Bleibt es dabei, „Aufträge erneut senden“.
- **„kein IBM-Zugang im Gerät“**: Die Präparatorin gibt Schlüssel und CRN noch einmal ein.
- **„Frist überschritten“ bei IBM**: kein Verlust. Später noch einmal „Briefkasten prüfen“; der
  IBM-Auftrag ist gemerkt.
- **Bestellung verfallen** (nach 2 Tagen): Der Ausgeber vergisst sie und bestellt neu. Das Pfand
  regelt die Kette.

## Ehrlich offen

- **Ein Knoten.** Vor einer Öffnung fragt die Präparatorin alle bekannten Knoten. Zurzeit ist das
  nur C1. Im Pilot vertrauen also alle diesem einen Knoten, was Finalität und Token betrifft. Ein
  zweiter Knoten kommt mit dem zweiten Validator (Launch-Checkliste Punkt 16).
- **Noch nie auf echten Geräten gelaufen.** Alles ist mit Tests geprüft, aber Dateiablage,
  SecureStore, IBM-Warteschlange und die Dauer einer Runde sieht erst der Pilot. Der erste
  Durchlauf ist deshalb auch eine Messung.
- **Keine Benachrichtigung.** Neue Aufträge sieht man nur, wenn man in die App schaut. Für den
  Pilot genügt eine Gruppe im Messenger, in der der Ausgeber Bescheid sagt.
- **IBM-Kontingent.** Kostenlos sind 10 Minuten je 28 Tage. Wie viel eine Messung verbraucht,
  misst der erste Lauf.
