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
- **Die frühere Schwäche ist behoben (W2c, #546):**
  - **Befund:** Die alte Toleranz von 15 % bei höchstens 64 Stellen war gegen einen
    optimalen Kloner gebrochen (14,6 % Fehler je Kopie). Eine geklonte Kopie bestand mit
    0,55.
  - **Jetzt:** Toleranz je Note und k_note aus der exakten Tabelle. Ein Fälscher kommt
    damit höchstens mit 2⁻⁴⁰ durch.

## B. Zuerst: die Prüfung deutlich stärker machen

**Stand 16.11. abends:** Die Punkte 1–4 und 7 sind in Spezifikation und Knoten
umgesetzt (W2c: Toleranz je Note, k_note aus der exakten Tabelle mit Absprache-Grenze
f = (t−1)/v, Offenlegen in Teilen, neue Grenzen). Offen sind noch Punkt 5 (mehrstufige
Prüfung) und Punkt 6 (Fehlerkorrektur messen).

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
7. ~~**Quantenmechanische Schranke**~~ – erledigt: Die Tabelle rechnet mit dem
   optimalen quantenmechanischen Kloner (14,64 %), nicht mehr mit einem ratenden Fälscher.
   Ob verschränkte Angriffe über mehrere Stellen mehr bringen, ist Frage 1 im Prüfauftrag
   (`docs/QUANTENGELD_PRUEFAUFTRAG.md`).

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
     W. `s_kette.py` steht (Stichproben-Annahmestellen, Wahl c auf der Kette). Die
     S-Vorführung (`--s-token`) ist gelaufen: ideal, im Rauschmodell und **auf echter
     IBM-Hardware** (0,46 % Fehler, angenommen; Doppelvorlage abgewiesen). Das Einlösen
     auf der Kette ist spezifiziert (`KetteS.einloesen`: Vorlage von der Nutzerin
     unterschrieben, Stichprobe gebunden, Anteile offengelegt). **Im Knoten (schlafend):**
     Regeln S2a und S2b Teil 1 (#547); die Einhängung in die Kette als Satzart im W-Modul
     (S2b Teil 2, `quanten_w_s_kette.go`): Ausgabe mit Bestätigung aller Validatoren,
     Bindung, Wahl, Anmeldung der bei der Ausgabe **festgelegten** Annahmestelle (sonst
     könnte ein Dritter die offengelegte Stichprobe aus der Warteschlange kopieren und
     zuerst einlösen), Offenlegen in Teilen, Frist, Wert über Anteile im W-Topf.
     **Offen:** Validator-Dienst für S (Offenlegen der Stichprobe), App, Vorlage 4
     abschicken (Nutzer).
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
2. ~~**Prüfer-Dienst für Validatoren**~~ – **gebaut (W2d, 16.11. abends,
   `quanten_w_validator.go`):**
   - verschlüsselter Tresor;
   - Prägebestätigung mit Prüfung der Blätter gegen die Wurzel;
   - Anteile erst nach der Antwort;
   - Hintergrund für Zufall, Offenlegen in Teilen über die Nachbarn (`QUANTEN_W_PEERS`)
     und Fristen.
3. **App:** eine Variante-W-Note und ihren Prüfstand anzeigen.
4. **Vor jeder Freischaltung (W6):**
   - Gutachten ohne offene kritische Punkte (W5).
   - Parameter aus Abschnitt B.
   - Pilot im Testnetz.

## E. Wartet auf dich (Freigaben und Konten)

- **IBM-Lauf `wiesner = fern` auf dem echten Chip:** Workflow „Quantengeld auf
  Quanten-Hardware“, Modus `ibm`. Den Lauf startet die Sitzung, die Freigabe gibst du.
- ~~**App 1.23.2**~~ – vollständig veröffentlicht (APK und IPA, `quelle.json` geprüft).
- **Konto für eine Ionenfalle** (Quantinuum oder IonQ), für C.6 und W1.
- **Anschreiben an den BMFTR-Verbund** verschicken (`ANSCHREIBEN.md`, Vorlage 2). Das ist
  W4 mit Speicher.
- **Gutachten:** Der Prüfauftrag (`docs/QUANTENGELD_PRUEFAUFTRAG.md`) ist auf dem Stand
  vom 16.11. abends (W2c, W2d, Variante S). Einen Gutachter suchen.
- **Außerdem offen:** Stufe-0-Schlüssel, eigene Registrierung (Stufe 1 scharf).
