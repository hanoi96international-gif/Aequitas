# Offene Punkte – Quantengeld (Stand 16.11.2026, Sitzungsende)

Hauptaugenmerk der nächsten Sitzung: **dezentrales Quantengeld, das unter heutigen
Umständen funktioniert**, damit Aequitas ernstzunehmendes Quantengeld ist. Die Regel
bleibt: Nichts wird behauptet, was nicht gemessen oder bewiesen ist (QUANTENSICHERHEIT.md).

## A. Stand (auf main)

- **Variante W:** privat prüfbar, klassische Prüfung, das Geheimnis liegt als Anteile
  bei den Validatoren.
  - Spezifikation: `wiesner_kette.py`.
  - Regeln im Knoten: `quanten_w.go`, `quanten_w_register.go` (#539).
  - Geschärft und eingehängt, schlafend: `quanten_w_kette.go` (#541, #542).
  - Prägen ohne Vertrauen: `note_fern`, `wiesner_praegen.py`.
  - Ablauf von Anfang bis Ende mit der Kette: `hardware_ibm.py --wiesner --kette`.
  - Workflow-Auswahl: `messen | fern | kette`.
  - Prüfauftrag W5: `docs/QUANTENGELD_PRUEFAUFTRAG.md`.
- **Echte Hardware (IBM `ibm_marrakesh`, HEUTE.md 3.3):**
  - **Ohne Warten:** Der ganze Ablauf läuft mit dezentraler Prüfung. Ergebnis: echt,
    10 AEQ beim Empfänger, Annahme 0,998.
  - **Mit 20 µs Halten und Teleportation:** abgewiesen, 39 % Fehler je Treffer.
  - **3-Qubit-Schutz:** kein messbarer Gewinn.
- **Ehrliche Grenze:** Geld zum Mitnehmen gibt es erst mit einem Quantenspeicher. Heute
  lebt eine Note Mikrosekunden.
- **Ehrliche Schwäche, noch nicht behoben:** Die Toleranz liegt bei 15 %, geprüft werden
  höchstens 64 Stellen (`K_MAX`). **Schlimmer als gedacht (Befund 16.11. abends,
  `wiesner_parameter.py`):** Ein optimaler Kloner macht zwei Kopien mit je 14,6 % Fehler,
  und das liegt unter 15 %. Eine geklonte Kopie besteht bei k = 64 mit 0,55. Die frühere
  Angabe (3,8·10⁻⁵) beruhte auf einem Fälscher, der nur rät.

## B. Zuerst: die Prüfung deutlich stärker machen

1. **Ziel:** Ein Fälscher besteht höchstens mit 2⁻⁴⁰ je Prüfung. Das gilt auch für eine
   Absprache, die einen Anteil f der Stellen kennt (bei Fern-Präparation f = 1/v).
   **Gerechnet (`wiesner_parameter.py`):** Bei e = 0,7 % (IBM ohne Warten) braucht es
   eine Toleranz von 4 % und k = 840. Die Toleranz muss immer deutlich unter 14,6 %
   liegen (Pastawski u. a. 2012).
2. **Toleranz an die gemessene Fehlerrate koppeln:**
   - τ liegt knapp über der Fehlerrate e der Hardware (aus den W1-Läufen), statt fest
     bei 15 %.
   - Daraus k (geprüfte Stellen) und n (Notengröße) exakt rechnen. Dafür
     `faelschen_wahrscheinlichkeit` um eine Tabelle e → τ, k, n erweitern.
3. **`K_MAX` anheben** (64 → einige Hundert) und, wo nötig, auch `N_MAX`:
   - Spezifikation, Knoten und Prüfvektor gleich ziehen, mit Tests.
   - Die Grenzen werden nur bewusst neu gesetzt, nie gelockert, um etwas grün zu
     bekommen.
4. **Mehr Validatoren je Note** (v bis 64): Jeder kennt dann nur einen kleinen Teil der
   Note (kleines f).
5. **Mehrstufige Prüfung:** Einlösen erst nach zwei unabhängigen Ziehungen oder nach
   einem sequenziellen Test. Die Absprache-Rechnung dafür aufstellen.
6. **Fehlerkorrektur je Stelle** (`hardware.praegen_geschuetzt`) gegen die Fehlerrate
   messen. Sie senkt e und damit τ.
7. **Quantenmechanische Schranke** (0,927ⁿ, Molina–Vidick–Watrous) zusätzlich zum
   klassischen Fälschermodell rechnen und in die Tabelle aufnehmen.

## C. Weiter analysieren: Lösungen, die ohne Quantenspeicher auskommen

Für jeden Weg gilt dasselbe Vorgehen: die Quelle selbst lesen (Angaben unten noch
ungeprüft), Annahmen und Sicherheit klären, dann entscheiden, was Aequitas davon heute
bauen kann.

1. **Quanten-Token ohne Quantenspeicher (Raumzeit-Token):**
   - Quelle: Kent, Lowndes, Pitalúa-García, Rarity, „Practical quantum tokens without
     quantum memories and experimental tests“ (npj Quantum Information 2022). Dazu
     gehört Kents „S-money“.
   - Idee: Der Halter misst die Zustände **sofort** und hält danach ein klassisches
     Token. Es gilt nur an genau einem der vorgesehenen Orte bzw. Zeitpunkte.
   - Möglicherweise **der heute machbare Weg**: QKD-Technik statt Speicher, Prüfung
     durch die Kette.
   - **Analysiert (16.11. abends, ANALYSE.md 5b „Variante S“):** Der Weg ist heute
     machbar, unfälschbar, privat und dezentral prüfbar, aber kein Hand-zu-Hand-Bargeld.
     Doppelvorlage per Breidbart-Messung: 14,6 % Fehler, also dieselbe Schwelle wie bei
     W. Als Nächstes kommen `s_kette.py` und die S-Vorführung auf IBM.
   - Zu klären: Lässt sich „Ort“ auf Aequitas als „Prüfer bzw. Region + Block-Fenster“
     abbilden, und wie viel Sicherheit bleibt dann übrig?
2. **Zustellen statt Aufbewahren:**
   - Die Note geht über ein QKD-Netz (EuroQCI-Testnetze, kommerzielle Strecken) direkt
     zum Prüfer oder Empfänger und wird dort sofort gemessen.
   - Das ergibt einen Quanten-Scheck, kein Bargeld.
   - Zu klären: Welche Strecken sind heute zugänglich?
3. **Praktisches Quantengeld mit schwachen Lichtpulsen:**
   - Quelle: Bozzio u. a., „Experimental investigation of practical unforgeable quantum
     money“ (npj QI 2018).
   - Vergleichen: ihre Rauschtoleranz und Verluste mit den Parametern von Variante W.
4. **Halb-quantisches Geld (Radian & Sattath):**
   - Bank und Prüfung sind rein klassisch, das passt zur dezentralen Prüfung.
   - Beim Halter braucht es weiter einen Quantenspeicher.
5. **Offline-Bargeld als Träger:**
   - Das Bargeld-Verhalten liefern heute Secure Element und Offline-Bargeld Stufe 1b.
   - Der Quanten-Teil steckt nur in Prägung und Einlösung.
   - Genau abgrenzen, was davon „Quantengeld“ heißen darf.
6. **Gemietete Speicherzeit in der Ionenfalle:**
   - Quantinuum und IonQ halten Qubits Sekunden bis Minuten.
   - Ablauf: Note in der Cloud prägen, halten, über die Kette übertragen, prüfen.
     „Mitnehmen“ heißt hier Verwahrung beim Betreiber, nachprüfbar über die Kette.
   - Dafür braucht es ein Konto.

**Ergebnis:** ein neuer Abschnitt in ANALYSE.md mit einer Empfehlung, was zuerst gebaut
wird.

## D. Bauen und messen

1. **W1 vervollständigen:**
   - IBM ist gemessen (siehe A).
   - Offen sind `fern` auf dem echten Chip und die Ionenfalle (Sekunden statt
     Mikrosekunden).
   - Aus den Fehlerraten folgen die Parameter für Abschnitt B.
2. **Prüfer-Dienst für Validatoren:** Anteile verwahren, offenlegen und bestätigen, als
   Gegenstück zur schlafenden Kettenseite.
3. **App:** eine Variante-W-Note und ihren Prüfstand anzeigen.
4. **Vor jeder Freischaltung (W6):**
   - Gutachten ohne offene kritische Punkte (W5).
   - Parameter aus Abschnitt B.
   - Pilot im Testnetz.

## E. Wartet auf dich (Freigaben und Konten)

- **IBM-Lauf `wiesner = fern` auf dem echten Chip:** Workflow „Quantengeld auf
  Quanten-Hardware“, Modus `ibm`. Den Lauf startet die Sitzung, die Freigabe gibst du.
- **App 1.23.2:** Die APK ist veröffentlicht. Die **iOS-IPA fehlt noch im Release**:
  release-ios mit Lauf 43884174136, danach `quelle.json` prüfen.
- **Konto für eine Ionenfalle** (Quantinuum oder IonQ), für C.6 und W1.
- **Anschreiben an den BMFTR-Verbund** verschicken (`ANSCHREIBEN.md`, Vorlage 2). Das ist
  W4 mit Speicher.
- **Außerdem offen:** Stufe-0-Schlüssel, eigene Registrierung (Stufe 1 scharf), Gutachten.
